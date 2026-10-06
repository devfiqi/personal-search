package indexer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/devfiqi/personal-search/core/internal/extractor"
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

	service := New(store, nil)
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

type fakePDFExtractor struct {
	result extractor.PDFResult
	err    error
}

func (fake fakePDFExtractor) ExtractPDF(context.Context, string) (extractor.PDFResult, error) {
	return fake.result, fake.err
}

func TestIndexFolderExtractsAndSearchesPDF(t *testing.T) {
	ctx := context.Background()
	temporaryDirectory := t.TempDir()
	documentsDirectory := filepath.Join(temporaryDirectory, "documents")
	if err := os.Mkdir(documentsDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(documentsDirectory, "database.pdf"), []byte("pdf fixture"), 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := storage.Open(filepath.Join(temporaryDirectory, "search.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	service := New(store, fakePDFExtractor{
		result: extractor.PDFResult{Text: "replication lag handbook", PageCount: 1},
	})
	report, err := service.IndexFolder(ctx, documentsDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if report.Indexed != 1 || len(report.Errors) != 0 {
		t.Fatalf("report = %+v", report)
	}

	results, err := store.Search(ctx, "replication lag", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Name != "database.pdf" {
		t.Fatalf("Search() results = %+v", results)
	}
}

func TestIndexFolderContinuesAfterPDFExtractionFailure(t *testing.T) {
	ctx := context.Background()
	temporaryDirectory := t.TempDir()
	documentsDirectory := filepath.Join(temporaryDirectory, "documents")
	if err := os.Mkdir(documentsDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(documentsDirectory, "notes.txt"), []byte("searchable notes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(documentsDirectory, "locked.pdf"), []byte("pdf fixture"), 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := storage.Open(filepath.Join(temporaryDirectory, "search.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	service := New(store, fakePDFExtractor{err: errors.New("encrypted_pdf: PDF requires a password")})
	report, err := service.IndexFolder(ctx, documentsDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if report.Indexed != 1 || len(report.Errors) != 1 {
		t.Fatalf("report = %+v", report)
	}

	results, err := store.Search(ctx, "searchable", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Name != "notes.txt" {
		t.Fatalf("Search() results = %+v", results)
	}
}
