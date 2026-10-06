package indexer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/devfiqi/personal-search/core/internal/storage"
)

func TestIndexFolderIndexesReusesAndRemovesDocuments(t *testing.T) {
	ctx := context.Background()
	temporaryDirectory := t.TempDir()
	documentsDirectory := filepath.Join(temporaryDirectory, "documents")
	if err := os.Mkdir(documentsDirectory, 0o700); err != nil {
		t.Fatal(err)
	}

	documentPath := filepath.Join(documentsDirectory, "notes.md")
	if err := os.WriteFile(documentPath, []byte("replication lag notes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(documentsDirectory, "photo.jpg"), []byte("ignored"), 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := storage.Open(filepath.Join(temporaryDirectory, "search.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	service := New(store)
	first, err := service.IndexFolder(ctx, documentsDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if first.Indexed != 1 || first.Skipped != 1 {
		t.Fatalf("first report = %+v", first)
	}

	second, err := service.IndexFolder(ctx, documentsDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if second.Unchanged != 1 {
		t.Fatalf("second report = %+v", second)
	}

	if err := os.WriteFile(documentPath, []byte("updated replication lag notes"), 0o600); err != nil {
		t.Fatal(err)
	}
	third, err := service.IndexFolder(ctx, documentsDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if third.Indexed != 1 {
		t.Fatalf("third report = %+v", third)
	}

	updatedResults, err := store.Search(ctx, "updated", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(updatedResults) != 1 {
		t.Fatalf("Search() returned %d updated results, want 1", len(updatedResults))
	}

	if err := os.Remove(documentPath); err != nil {
		t.Fatal(err)
	}
	fourth, err := service.IndexFolder(ctx, documentsDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if fourth.Removed != 1 {
		t.Fatalf("fourth report = %+v", fourth)
	}

	results, err := store.Search(ctx, "replication", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("Search() returned %d results after removal", len(results))
	}
}
