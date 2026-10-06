package indexer

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/devfiqi/personal-search/core/internal/storage"
	"github.com/fsnotify/fsnotify"
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
	PendingScan  bool      `json:"pending_scan"`
	WatchedPaths int       `json:"watched_paths"`
}

type Manager struct {
	indexer  *Indexer
	folders  FolderStore
	debounce time.Duration

	mutex       sync.RWMutex
	paused      bool
	indexing    bool
	dirty       bool
	lastIndexed time.Time
	lastError   string
	watchCount  int

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
		wake: make(chan struct{}, 1), reload: make(chan struct{}, 1), done: make(chan struct{}),
	}
}

func (manager *Manager) Start(parent context.Context) error {
	var startError error
	manager.startOnce.Do(func() {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			startError = fmt.Errorf("start folder watcher: %w", err)
			return
		}
		ctx, cancel := context.WithCancel(parent)
		manager.cancel = cancel
		if err := manager.reloadWatches(ctx, watcher); err != nil {
			watcher.Close()
			cancel()
			startError = err
			return
		}
		go manager.run(ctx, watcher)
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
		LastError: manager.lastError, PendingScan: manager.dirty, WatchedPaths: manager.watchCount,
	}
}

func (manager *Manager) run(ctx context.Context, watcher *fsnotify.Watcher) {
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
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Create|fsnotify.Rename) != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					_ = manager.addRecursive(watcher, event.Name)
				}
			}
			manager.RequestScan()
		case err, ok := <-watcher.Errors:
			if ok {
				manager.recordError(fmt.Errorf("watch folders: %w", err))
			}
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
	for _, folder := range folders {
		if _, err := manager.indexer.IndexFolder(ctx, folder.Path); err != nil {
			manager.recordError(err)
		}
		if ctx.Err() != nil {
			return
		}
	}
}

func (manager *Manager) reloadWatches(ctx context.Context, watcher *fsnotify.Watcher) error {
	for _, watched := range watcher.WatchList() {
		_ = watcher.Remove(watched)
	}
	folders, err := manager.folders.ListFolders(ctx)
	if err != nil {
		return err
	}
	for _, folder := range folders {
		if err := manager.addRecursive(watcher, folder.Path); err != nil {
			manager.recordError(err)
		}
	}
	manager.mutex.Lock()
	manager.watchCount = len(watcher.WatchList())
	manager.mutex.Unlock()
	return nil
}

func (manager *Manager) addRecursive(watcher *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkError error) error {
		if walkError != nil {
			return nil
		}
		if entry.IsDir() {
			if err := watcher.Add(path); err != nil {
				return fmt.Errorf("watch folder: %w", err)
			}
		}
		return nil
	})
}

func (manager *Manager) recordError(err error) {
	manager.mutex.Lock()
	manager.lastError = err.Error()
	manager.mutex.Unlock()
}
