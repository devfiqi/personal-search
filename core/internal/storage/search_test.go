package storage

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"
)

func TestSearchPrefersAllTermsAndFallsBackToPartialMatches(t *testing.T) {
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
	documents := []Document{
		{FolderID: folderID, Path: "/documents/both.md", Name: "both.md", Extension: ".md", Content: "replication lag", Status: "indexed"},
		{FolderID: folderID, Path: "/documents/partial.md", Name: "partial.md", Extension: ".md", Content: "replication notes", Status: "indexed"},
	}
	for _, document := range documents {
		if err := store.UpsertDocument(ctx, document); err != nil {
			t.Fatal(err)
		}
	}

	results, err := store.Search(ctx, "replication lag", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("Search() returned %d results, want 2", len(results))
	}
	if results[0].Name != "both.md" || results[1].Name != "partial.md" {
		t.Fatalf("Search() order = %q, %q", results[0].Name, results[1].Name)
	}
}

func TestSearchReturnsBroadMatchesWithoutDroppingNameMatches(t *testing.T) {
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
	for index := 0; index < 250; index++ {
		name := "document-" + strconv.Itoa(index) + ".txt"
		if index == 0 {
			name = "linkedin-notes.txt"
		}
		if err := store.UpsertDocument(ctx, Document{
			FolderID:  folderID,
			Path:      "/documents/" + name,
			Name:      name,
			Extension: ".txt",
			Content:   "linkedin reference material",
			Status:    "indexed",
		}); err != nil {
			t.Fatal(err)
		}
	}

	results, err := store.Search(ctx, "linkedin", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 50 {
		t.Fatalf("Search() returned %d results, want 50", len(results))
	}
	if results[0].Name != "linkedin-notes.txt" {
		t.Fatalf("top result = %q, want filename match first", results[0].Name)
	}
}

func TestSearchTreatsOperatorsAsText(t *testing.T) {
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
	if err := store.UpsertDocument(ctx, Document{
		FolderID:  folderID,
		Path:      "/documents/query.txt",
		Name:      "query.txt",
		Extension: ".txt",
		Content:   "notes about OR and NEAR operators",
		Status:    "indexed",
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Search(ctx, `OR NEAR "`, 10); err != nil {
		t.Fatalf("Search() returned an error for operator input: %v", err)
	}
}

func TestUpdatingDocumentReplacesIndexedContent(t *testing.T) {
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
		Path:      "/documents/changing.txt",
		Name:      "changing.txt",
		Extension: ".txt",
		Content:   "original keyword",
		Status:    "indexed",
	}
	if err := store.UpsertDocument(ctx, document); err != nil {
		t.Fatal(err)
	}

	document.Content = "replacement keyword"
	if err := store.UpsertDocument(ctx, document); err != nil {
		t.Fatal(err)
	}

	originalResults, err := store.Search(ctx, "original", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(originalResults) != 0 {
		t.Fatalf("Search() returned %d stale results", len(originalResults))
	}

	replacementResults, err := store.Search(ctx, "replacement", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(replacementResults) != 1 {
		t.Fatalf("Search() returned %d replacement results, want 1", len(replacementResults))
	}
}

func TestRemovingFolderDeletesDerivedDocuments(t *testing.T) {
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
	if err := store.UpsertDocument(ctx, Document{
		FolderID: folderID, Path: "/documents/private.txt", Name: "private.txt",
		Extension: ".txt", Content: "private keyword", Status: "indexed",
	}); err != nil {
		t.Fatal(err)
	}

	removed, err := store.RemoveFolder(ctx, "/documents")
	if err != nil || !removed {
		t.Fatalf("RemoveFolder() = %v, %v", removed, err)
	}
	results, err := store.Search(ctx, "private", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("Search() returned %d results after folder removal", len(results))
	}
}

func TestResetDeletesFoldersDocumentsAndFailures(t *testing.T) {
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
	if err := store.UpsertDocument(ctx, Document{
		FolderID: folderID, Path: "/documents/broken.pdf", Name: "broken.pdf",
		Extension: ".pdf", Status: "error", Error: "damaged",
	}); err != nil {
		t.Fatal(err)
	}
	failures, err := store.ListFailures(ctx, 10)
	if err != nil || len(failures) != 1 {
		t.Fatalf("ListFailures() = %+v, %v", failures, err)
	}

	if err := store.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	folders, err := store.ListFolders(ctx)
	if err != nil || len(folders) != 0 {
		t.Fatalf("ListFolders() = %+v, %v", folders, err)
	}
	failures, err = store.ListFailures(ctx, 10)
	if err != nil || len(failures) != 0 {
		t.Fatalf("ListFailures() after reset = %+v, %v", failures, err)
	}
}
