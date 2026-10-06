package storage

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSemanticChunksCanBeReadAndMarkedIndexed(t *testing.T) {
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
		Path:      "/documents/example.txt",
		Name:      "example.txt",
		Extension: ".txt",
		Content:   "A searchable local semantic passage.",
		Status:    "indexed",
	}
	if err := store.UpsertDocument(ctx, document); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureDocumentChunks(ctx, document.Path); err != nil {
		t.Fatal(err)
	}

	pending, err := store.ListPendingSemanticChunks(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Text != document.Content {
		t.Fatalf("pending semantic chunks = %+v", pending)
	}
	if err := store.MarkSemanticChunksIndexed(ctx, []int64{pending[0].ID}); err != nil {
		t.Fatal(err)
	}
	chunks, err := store.SemanticChunksByIDs(ctx, []int64{pending[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if chunks[pending[0].ID].Path != document.Path {
		t.Fatalf("semantic chunk = %+v", chunks[pending[0].ID])
	}
}
