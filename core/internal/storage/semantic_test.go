package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureDocumentChunksTracksContentAndClearsFailures(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "search.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	folderID, err := store.RegisterFolder(ctx, "/documents")
	if err != nil {
		t.Fatal(err)
	}
	document := Document{
		FolderID:  folderID,
		Path:      "/documents/semantic.txt",
		Name:      "semantic.txt",
		Extension: ".txt",
		Content:   strings.Repeat("searchable semantic passage ", 200),
		Status:    "indexed",
	}
	if err := store.UpsertDocument(ctx, document); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureDocumentChunks(ctx, document.Path); err != nil {
		t.Fatal(err)
	}

	var count int
	var pending int
	if err := store.database.QueryRowContext(ctx, `
SELECT COUNT(*), SUM(vector_state = 'pending') FROM semantic_chunks`).Scan(&count, &pending); err != nil {
		t.Fatal(err)
	}
	if count < 2 || pending != count {
		t.Fatalf("semantic chunk state = count %d, pending %d", count, pending)
	}

	document.Content = ""
	document.Status = "error"
	document.Error = "unreadable"
	if err := store.UpsertDocument(ctx, document); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureDocumentChunks(ctx, document.Path); err != nil {
		t.Fatal(err)
	}
	if err := store.database.QueryRowContext(ctx, "SELECT COUNT(*) FROM semantic_chunks").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("semantic chunks after extraction failure = %d, want 0", count)
	}
}
