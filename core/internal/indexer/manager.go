package indexer

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/devfiqi/personal-search/core/internal/storage"
)

const defaultDebounce = 350 * time.Millisecond

type FolderStore interface {
	ListFolders(ctx context.Context) ([]storage.Folder, error)
}

type ManagerState struct {
	Paused       bool      `json:"paused"`
	Indexing     bool      `json:"indexing"`
	LastIndexed  time.Time `json:"last_indexed,omitempty"`
	LastError    string    `json:"last_error,omitempty"`
	WatchError   string    `json:"watch_error,omitempty"`
	PendingScan  bool      `json:"pending_scan"`
	WatchedPaths int       `json:"watched_paths"`
}

type Manager struct {
	indexer  *Indexer
	folders  FolderStore
	debounce time.Duration

	mutex        sync.RWMutex
	paused       bool
	indexing     bool
	dirty        bool
	fullScan     bool
	changedPaths map[string]struct{}
	lastIndexed  time.Time
	lastError    string
	watchError   string
	watchCount   int

	scanMutex sync.Mutex
	wake      chan struct{}
	reload    chan struct{}
	done      chan struct{}
	cancel    context.CancelFunc
	startOnce sync.Once
	stopOnce  sync.Once
}

func NewManager(indexer *Indexer, folders FolderStore) *Manager {
	return newManager(indexer, folders, defaultDebounce)
}

func newManager(indexer *Indexer, folders FolderStore, debounce time.Duration) *Manager {
	return &Manager{
		indexer: indexer, folders: folders, debounce: debounce,
		changedPaths: make(map[string]struct{}),
		wake:         make(chan struct{}, 1), reload: make(chan struct{}, 1), done: make(chan struct{}),
	}
}

func (manager *Manager) Start(parent context.Context) error {
	var startError error
	manager.startOnce.Do(func() {
		watcher, err := newFolderWatcher()
		if err != nil {
			startError = err
			return
		}
		ctx, cancel := context.WithCancel(parent)
		manager.cancel = cancel
		go manager.run(ctx, watcher)
		if err := manager.reloadWatches(ctx, watcher); err != nil {
			cancel()
			startError = err
			return
		}
		manager.RequestScan()
	})
	return startError
}

func (manager *Manager) Close() {
	manager.stopOnce.Do(func() {
		if manager.cancel == nil {
			return
		}
		manager.cancel()
		<-manager.done
	})
}

func (manager *Manager) RequestScan() {
	manager.mutex.Lock()
	manager.dirty = true
	manager.fullScan = true
	manager.mutex.Unlock()
	select {
	case manager.wake <- struct{}{}:
	default:
	}
}

func (manager *Manager) requestPath(path string) {
	manager.mutex.Lock()
	manager.dirty = true
	if path == "" {
		manager.fullScan = true
	} else {
		manager.changedPaths[filepath.Clean(path)] = struct{}{}
	}
	manager.mutex.Unlock()
	select {
	case manager.wake <- struct{}{}:
	default:
	}
}

func (manager *Manager) ReloadFolders() {
	select {
	case manager.reload <- struct{}{}:
	default:
	}
	manager.RequestScan()
}

func (manager *Manager) SetPaused(paused bool) {
	manager.mutex.Lock()
	manager.paused = paused
	manager.mutex.Unlock()
	if !paused {
		manager.RequestScan()
	}
}

func (manager *Manager) State() ManagerState {
	manager.mutex.RLock()
	defer manager.mutex.RUnlock()
	return ManagerState{
		Paused: manager.paused, Indexing: manager.indexing, LastIndexed: manager.lastIndexed,
		LastError: manager.lastError, WatchError: manager.watchError, PendingScan: manager.dirty,
		WatchedPaths: manager.watchCount,
	}
}

func (manager *Manager) run(ctx context.Context, watcher folderWatcher) {
	defer close(manager.done)
	defer watcher.Close()

	var timer *time.Timer
	var timerChannel <-chan time.Time
	schedule := func() {
		if timer == nil {
			timer = time.NewTimer(manager.debounce)
		} else {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(manager.debounce)
		}
		timerChannel = timer.C
	}

	for {
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case <-manager.wake:
			schedule()
		case <-manager.reload:
			if err := manager.reloadWatches(ctx, watcher); err != nil {
				manager.recordError(err)
			}
		case path := <-watcher.Events():
			if watcher.Overflowed() {
				path = ""
			}
			manager.requestPath(path)
		case <-timerChannel:
			timerChannel = nil
			manager.scan(ctx)
		}
	}
}

func (manager *Manager) scan(ctx context.Context) {
	manager.mutex.Lock()
	if manager.paused || !manager.dirty {
		manager.mutex.Unlock()
		return
	}
	manager.dirty = false
	fullScan := manager.fullScan
	changedPaths := make([]string, 0, len(manager.changedPaths))
	for path := range manager.changedPaths {
		changedPaths = append(changedPaths, path)
	}
	manager.fullScan = false
	manager.changedPaths = make(map[string]struct{})
	manager.indexing = true
	manager.lastError = ""
	manager.mutex.Unlock()

	manager.scanMutex.Lock()
	defer manager.scanMutex.Unlock()
	defer func() {
		manager.mutex.Lock()
		manager.indexing = false
		manager.lastIndexed = time.Now()
		manager.mutex.Unlock()
	}()

	folders, err := manager.folders.ListFolders(ctx)
	if err != nil {
		manager.recordError(err)
		return
	}
	if fullScan {
		for _, folder := range folders {
			if _, err := manager.indexer.IndexFolder(ctx, folder.Path); err != nil {
				manager.recordError(err)
			}
			if ctx.Err() != nil {
				return
			}
		}
		return
	}

	sort.Strings(changedPaths)
	for _, path := range changedPaths {
		folder, ok := folderForPath(folders, path)
		if !ok {
			continue
		}
		if _, err := manager.indexer.IndexPath(ctx, folder.Path, path); err != nil {
			manager.recordError(err)
		}
		if ctx.Err() != nil {
			return
		}
	}
}

func folderForPath(folders []storage.Folder, path string) (storage.Folder, bool) {
	for _, folder := range folders {
		relative, err := filepath.Rel(folder.Path, path)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return folder, true
		}
	}
	return storage.Folder{}, false
}

func (manager *Manager) reloadWatches(ctx context.Context, watcher folderWatcher) error {
	folders, err := manager.folders.ListFolders(ctx)
	if err != nil {
		return err
	}
	paths := make([]string, 0, len(folders))
	for _, folder := range folders {
		paths = append(paths, folder.Path)
	}
	watchError := watcher.Replace(paths)
	manager.mutex.Lock()
	manager.watchCount = watcher.Count()
	if watchError != nil {
		manager.watchError = watchError.Error()
	} else {
		manager.watchError = ""
	}
	manager.mutex.Unlock()
	return nil
}

func (manager *Manager) recordError(err error) {
	manager.mutex.Lock()
	manager.lastError = err.Error()
	manager.mutex.Unlock()
}
