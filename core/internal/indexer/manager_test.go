package indexer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devfiqi/personal-search/core/internal/storage"
)

func TestManagerIndexesChangesAndHonorsPause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	root := t.TempDir()
	documents := filepath.Join(root, "documents")
	if err := os.Mkdir(documents, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(filepath.Join(root, "search.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.RegisterFolder(ctx, documents); err != nil {
		t.Fatal(err)
	}

	manager := newManager(New(store, nil), store, 25*time.Millisecond)
	if err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	firstPath := filepath.Join(documents, "first.txt")
	if err := os.WriteFile(firstPath, []byte("orchid notes"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager.requestPath(firstPath)
	waitForResult(t, store, "orchid", 1)

	manager.SetPaused(true)
	secondPath := filepath.Join(documents, "second.txt")
	if err := os.WriteFile(secondPath, []byte("saffron notes"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager.requestPath(secondPath)
	time.Sleep(150 * time.Millisecond)
	assertResultCount(t, store, "saffron", 0)

	manager.SetPaused(false)
	waitForResult(t, store, "saffron", 1)
	if err := os.Remove(firstPath); err != nil {
		t.Fatal(err)
	}
	manager.requestPath(firstPath)
	waitForResult(t, store, "orchid", 0)
}

func TestManagerWatchesFolderWithMissingEntry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	root := t.TempDir()
	documents := filepath.Join(root, "documents")
	nested := filepath.Join(documents, "nested")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	notesPath := filepath.Join(documents, "notes.txt")
	if err := os.WriteFile(notesPath, []byte("alpha notes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/tmp/personal-search-missing-target", filepath.Join(documents, "Microsoft")); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(filepath.Join(root, "search.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.RegisterFolder(ctx, documents); err != nil {
		t.Fatal(err)
	}

	manager := newManager(New(store, nil), store, 25*time.Millisecond)
	if err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	waitForResult(t, store, "alpha", 1)
	if err := os.WriteFile(notesPath, []byte("alpha notes revised token"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager.requestPath(notesPath)
	waitForResult(t, store, "revised", 1)
	extraPath := filepath.Join(nested, "extra.txt")
	if err := os.WriteFile(extraPath, []byte("nested zephyr token"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager.requestPath(extraPath)
	waitForResult(t, store, "zephyr", 1)

	state := manager.State()
	if state.WatchedPaths == 0 {
		t.Fatal("selected folder is not being watched")
	}
	if state.WatchError != "" {
		t.Fatalf("watch error = %q", state.WatchError)
	}
}

func waitForResult(t *testing.T, store *storage.Store, query string, count int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		results, err := store.Search(context.Background(), query, 10)
		if err == nil && len(results) == count {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	assertResultCount(t, store, query, count)
}

func assertResultCount(t *testing.T, store *storage.Store, query string, count int) {
	t.Helper()
	results, err := store.Search(context.Background(), query, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != count {
		t.Fatalf("Search(%q) returned %d results, want %d", query, len(results), count)
	}
}
