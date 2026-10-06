package indexer

// folderWatcher reports paths that changed beneath selected folder trees.
type folderWatcher interface {
	Events() <-chan string
	Overflowed() bool
	Replace(paths []string) error
	Count() int
	Close() error
}
