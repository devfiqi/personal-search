package indexer

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/devfiqi/personal-search/core/internal/extractor"
	"github.com/devfiqi/personal-search/core/internal/storage"
)

const MaxTextFileBytes int64 = 10 * 1024 * 1024
const MaxPDFFileBytes int64 = 250 * 1024 * 1024

var supportedExtensions = map[string]struct{}{
	".c": {}, ".cc": {}, ".cpp": {}, ".css": {}, ".go": {}, ".h": {},
	".hpp": {}, ".html": {}, ".java": {}, ".js": {}, ".json": {},
	".jsx": {}, ".md": {}, ".markdown": {}, ".php": {}, ".py": {},
	".rb": {}, ".rs": {}, ".sh": {}, ".sql": {}, ".swift": {},
	".toml": {}, ".ts": {}, ".tsx": {}, ".txt": {}, ".xml": {},
	".yaml": {}, ".yml": {}, ".zsh": {},
}

const pdfExtension = ".pdf"

type Store interface {
	RegisterFolder(ctx context.Context, path string) (int64, error)
	DocumentIsCurrent(ctx context.Context, path string, sizeBytes int64, modifiedAtNS int64) (bool, error)
	UpsertDocument(ctx context.Context, document storage.Document) error
	DeleteMissingDocuments(ctx context.Context, folderID int64, seen map[string]struct{}) (int, error)
}

type Indexer struct {
	store        Store
	pdfExtractor PDFExtractor
}

type PDFExtractor interface {
	ExtractPDF(ctx context.Context, path string) (extractor.PDFResult, error)
}

type FileError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

type Report struct {
	Folder    string      `json:"folder"`
	Indexed   int         `json:"indexed"`
	Unchanged int         `json:"unchanged"`
	Removed   int         `json:"removed"`
	Skipped   int         `json:"skipped"`
	Errors    []FileError `json:"errors"`
}

func New(store Store, pdfExtractor PDFExtractor) *Indexer {
	return &Indexer{store: store, pdfExtractor: pdfExtractor}
}

func (indexer *Indexer) IndexFolder(ctx context.Context, root string) (Report, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return Report{}, fmt.Errorf("resolve folder path: %w", err)
	}
	absoluteRoot = filepath.Clean(absoluteRoot)

	info, err := os.Stat(absoluteRoot)
	if err != nil {
		return Report{}, fmt.Errorf("read folder: %w", err)
	}
	if !info.IsDir() {
		return Report{}, fmt.Errorf("index path is not a folder: %s", absoluteRoot)
	}

	folderID, err := indexer.store.RegisterFolder(ctx, absoluteRoot)
	if err != nil {
		return Report{}, err
	}

	report := Report{Folder: absoluteRoot, Errors: []FileError{}}
	seen := make(map[string]struct{})

	err = filepath.WalkDir(absoluteRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			report.Errors = append(report.Errors, FileError{Path: path, Message: walkErr.Error()})
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			report.Skipped++
			return nil
		}

		extension := strings.ToLower(filepath.Ext(entry.Name()))
		_, isText := supportedExtensions[extension]
		if !isText && extension != pdfExtension {
			report.Skipped++
			return nil
		}
		seen[path] = struct{}{}

		fileInfo, err := entry.Info()
		if err != nil {
			report.Errors = append(report.Errors, FileError{Path: path, Message: err.Error()})
			return nil
		}

		current, err := indexer.store.DocumentIsCurrent(ctx, path, fileInfo.Size(), fileInfo.ModTime().UnixNano())
		if err != nil {
			return err
		}
		if current {
			report.Unchanged++
			return nil
		}

		document := storage.Document{
			FolderID:     folderID,
			Path:         path,
			Name:         entry.Name(),
			Extension:    extension,
			SizeBytes:    fileInfo.Size(),
			ModifiedAtNS: fileInfo.ModTime().UnixNano(),
			Status:       "indexed",
		}

		document.Content, err = indexer.extractContent(ctx, path, extension, fileInfo.Size())
		if err != nil {
			document.Status = "error"
			document.Error = err.Error()
		}

		if err := indexer.store.UpsertDocument(ctx, document); err != nil {
			return err
		}
		if document.Status == "error" {
			report.Errors = append(report.Errors, FileError{Path: path, Message: document.Error})
		} else {
			report.Indexed++
		}
		return nil
	})
	if err != nil {
		return Report{}, fmt.Errorf("walk folder: %w", err)
	}

	report.Removed, err = indexer.store.DeleteMissingDocuments(ctx, folderID, seen)
	if err != nil {
		return Report{}, err
	}
	return report, nil
}

func (indexer *Indexer) extractContent(ctx context.Context, path string, extension string, sizeBytes int64) (string, error) {
	if extension == pdfExtension {
		if sizeBytes > MaxPDFFileBytes {
			return "", fmt.Errorf("PDF exceeds extraction limit")
		}
		if indexer.pdfExtractor == nil {
			return "", fmt.Errorf("PDF extractor is unavailable")
		}
		result, err := indexer.pdfExtractor.ExtractPDF(ctx, path)
		if err != nil {
			return "", err
		}
		return result.Text, nil
	}

	if sizeBytes > MaxTextFileBytes {
		return "", fmt.Errorf("file exceeds text indexing limit")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(content) {
		return "", fmt.Errorf("file is not valid UTF-8")
	}
	return string(content), nil
}
