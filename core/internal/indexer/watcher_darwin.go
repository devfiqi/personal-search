//go:build darwin

package indexer

/*
#cgo LDFLAGS: -framework CoreServices
#include <CoreServices/CoreServices.h>
#include <dispatch/dispatch.h>
#include <stdint.h>
#include <stdlib.h>

extern void personalSearchFSEvent(void *info, char *path);

static void personalSearchStreamCallback(
	ConstFSEventStreamRef stream,
	void *info,
	size_t numEvents,
	void *eventPaths,
	const FSEventStreamEventFlags eventFlags[],
	const FSEventStreamEventId eventIds[]) {
	(void)stream;
	(void)eventFlags;
	(void)eventIds;
	char **paths = (char **)eventPaths;
	for (size_t index = 0; index < numEvents; index++) {
		personalSearchFSEvent(info, paths[index]);
	}
}

static FSEventStreamRef personalSearchStartStream(char **paths, int count, uintptr_t info, dispatch_queue_t *queueOut) {
	CFMutableArrayRef watched = CFArrayCreateMutable(NULL, count, &kCFTypeArrayCallBacks);
	if (watched == NULL) {
		return NULL;
	}
	for (int index = 0; index < count; index++) {
		CFStringRef path = CFStringCreateWithCString(NULL, paths[index], kCFStringEncodingUTF8);
		if (path == NULL) {
			CFRelease(watched);
			return NULL;
		}
		CFArrayAppendValue(watched, path);
		CFRelease(path);
	}

	FSEventStreamContext context = {0, (void *)info, NULL, NULL, NULL};
	FSEventStreamRef stream = FSEventStreamCreate(
		NULL,
		personalSearchStreamCallback,
		&context,
		watched,
		kFSEventStreamEventIdSinceNow,
		0.05,
		kFSEventStreamCreateFlagFileEvents | kFSEventStreamCreateFlagNoDefer | kFSEventStreamCreateFlagWatchRoot
	);
	CFRelease(watched);
	if (stream == NULL) {
		return NULL;
	}

	dispatch_queue_t queue = dispatch_queue_create("dev.fiqi.personal-search.fsevents", DISPATCH_QUEUE_SERIAL);
	FSEventStreamSetDispatchQueue(stream, queue);
	if (!FSEventStreamStart(stream)) {
		FSEventStreamInvalidate(stream);
		FSEventStreamRelease(stream);
		dispatch_release(queue);
		return NULL;
	}
	*queueOut = queue;
	return stream;
}

static void personalSearchStopStream(FSEventStreamRef stream, dispatch_queue_t queue) {
	if (stream == NULL) {
		return;
	}
	FSEventStreamStop(stream);
	FSEventStreamInvalidate(stream);
	FSEventStreamRelease(stream);
	if (queue != NULL) {
		dispatch_release(queue);
	}
}
*/
import "C"

import (
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"unsafe"
)

// macOS kqueue watches open every file and abandon the directory when one
// entry cannot be opened. FSEvents watches the folder tree without that
// failure, including edits to files that are already indexed.
var darwinWatchers sync.Map

type darwinWatcher struct {
	id       uintptr
	events   chan string
	overflow atomic.Bool

	lifecycle sync.Mutex
	mutex     sync.Mutex
	stream    C.FSEventStreamRef
	queue     C.dispatch_queue_t
	count     int
	closed    bool
}

func newFolderWatcher() (folderWatcher, error) {
	watcher := &darwinWatcher{
		id:     uintptr(atomic.AddUintptr(&nextDarwinWatcherID, 1)),
		events: make(chan string, 256),
	}
	darwinWatchers.Store(watcher.id, watcher)
	return watcher, nil
}

var nextDarwinWatcherID uintptr

//export personalSearchFSEvent
func personalSearchFSEvent(info unsafe.Pointer, path *C.char) {
	value, ok := darwinWatchers.Load(uintptr(info))
	if !ok {
		return
	}
	eventPath := ""
	if path != nil {
		eventPath = C.GoString(path)
	}
	value.(*darwinWatcher).notify(eventPath)
}

func (watcher *darwinWatcher) Events() <-chan string { return watcher.events }

func (watcher *darwinWatcher) Overflowed() bool { return watcher.overflow.Swap(false) }

func (watcher *darwinWatcher) Replace(paths []string) error {
	watcher.lifecycle.Lock()
	defer watcher.lifecycle.Unlock()

	usable := make([]string, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			continue
		}
		usable = append(usable, path)
	}

	watcher.mutex.Lock()
	if watcher.closed {
		watcher.mutex.Unlock()
		return errors.New("folder watcher is closed")
	}
	stream, queue := watcher.stream, watcher.queue
	watcher.stream = nil
	watcher.queue = nil
	watcher.mutex.Unlock()
	if stream != nil {
		C.personalSearchStopStream(stream, queue)
	}

	if len(usable) == 0 {
		watcher.mutex.Lock()
		watcher.count = 0
		watcher.mutex.Unlock()
		if len(paths) > 0 {
			return errors.New("selected folders could not be watched")
		}
		return nil
	}

	cPaths := make([]*C.char, len(usable))
	for index, path := range usable {
		cPaths[index] = C.CString(path)
	}
	defer func() {
		for _, path := range cPaths {
			C.free(unsafe.Pointer(path))
		}
	}()

	var startedQueue C.dispatch_queue_t
	started := C.personalSearchStartStream(&cPaths[0], C.int(len(cPaths)), C.uintptr_t(watcher.id), &startedQueue)
	watcher.mutex.Lock()
	closed := watcher.closed
	if closed || started == nil {
		watcher.count = 0
		watcher.mutex.Unlock()
		if started != nil {
			C.personalSearchStopStream(started, startedQueue)
		}
		if closed {
			return errors.New("folder watcher is closed")
		}
		return errors.New("selected folders could not be watched")
	}
	watcher.stream = started
	watcher.queue = startedQueue
	watcher.count = len(usable)
	watcher.mutex.Unlock()
	if len(usable) != len(paths) {
		return errors.New("one or more selected folders could not be watched")
	}
	return nil
}

func (watcher *darwinWatcher) Count() int {
	watcher.mutex.Lock()
	defer watcher.mutex.Unlock()
	return watcher.count
}

func (watcher *darwinWatcher) Close() error {
	watcher.lifecycle.Lock()
	defer watcher.lifecycle.Unlock()

	watcher.mutex.Lock()
	stream, queue := watcher.stream, watcher.queue
	watcher.stream = nil
	watcher.queue = nil
	watcher.count = 0
	watcher.closed = true
	watcher.mutex.Unlock()
	darwinWatchers.Delete(watcher.id)
	if stream != nil {
		C.personalSearchStopStream(stream, queue)
	}
	return nil
}

func (watcher *darwinWatcher) notify(path string) {
	watcher.mutex.Lock()
	closed := watcher.closed
	watcher.mutex.Unlock()
	if closed {
		return
	}
	select {
	case watcher.events <- path:
	default:
		watcher.overflow.Store(true)
	}
}
