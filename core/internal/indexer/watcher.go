package indexer

// folderWatcher reports that a selected folder tree changed.
// Events are coalesced: a receive means one or more paths changed.
type folderWatcher interface {
	Events() <-chan struct{}
	Replace(paths []string) error
	Count() int
	Close() error
}
