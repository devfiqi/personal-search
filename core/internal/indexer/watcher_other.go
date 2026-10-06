//go:build !darwin

package indexer

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
)

type fsnotifyWatcher struct {
	watcher *fsnotify.Watcher
	events  chan struct{}

	mutex  sync.Mutex
	count  int
	closed bool
	done   chan struct{}
}

func newFolderWatcher() (folderWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	folderWatcher := &fsnotifyWatcher{
		watcher: watcher,
		events:  make(chan struct{}, 1),
		done:    make(chan struct{}),
	}
	go folderWatcher.forward()
	return folderWatcher, nil
}

func (watcher *fsnotifyWatcher) Events() <-chan struct{} { return watcher.events }

func (watcher *fsnotifyWatcher) Replace(paths []string) error {
	watcher.mutex.Lock()
	if watcher.closed {
		watcher.mutex.Unlock()
		return errors.New("folder watcher is closed")
	}
	watcher.mutex.Unlock()

	for _, watched := range watcher.watcher.WatchList() {
		_ = watcher.watcher.Remove(watched)
	}
	var watchError error
	for _, path := range paths {
		if err := watcher.addTree(path); err != nil && watchError == nil {
			watchError = err
		}
	}
	watcher.mutex.Lock()
	watcher.count = len(watcher.watcher.WatchList())
	watcher.mutex.Unlock()
	if watchError != nil {
		return errors.New("one or more selected folders could not be watched")
	}
	return nil
}

func (watcher *fsnotifyWatcher) addTree(root string) error {
	var watchError error
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkError error) error {
		if walkError != nil || entry == nil || !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if err := watcher.watcher.Add(path); err != nil && watchError == nil {
			watchError = err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return watchError
}

func (watcher *fsnotifyWatcher) Count() int {
	watcher.mutex.Lock()
	defer watcher.mutex.Unlock()
	return watcher.count
}

func (watcher *fsnotifyWatcher) Close() error {
	watcher.mutex.Lock()
	if watcher.closed {
		watcher.mutex.Unlock()
		return nil
	}
	watcher.closed = true
	watcher.count = 0
	watcher.mutex.Unlock()
	err := watcher.watcher.Close()
	<-watcher.done
	return err
}

func (watcher *fsnotifyWatcher) forward() {
	defer close(watcher.done)
	for {
		select {
		case _, ok := <-watcher.watcher.Events:
			if !ok {
				return
			}
			watcher.notify()
		case _, ok := <-watcher.watcher.Errors:
			if !ok {
				return
			}
		}
	}
}

func (watcher *fsnotifyWatcher) notify() {
	watcher.mutex.Lock()
	closed := watcher.closed
	watcher.mutex.Unlock()
	if closed {
		return
	}
	select {
	case watcher.events <- struct{}{}:
	default:
	}
}
