package indexer

import (
	"context"
	"errors"
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
	EnsureDocumentChunks(ctx context.Context, path string) error
	DeleteMissingDocuments(ctx context.Context, folderID int64, seen map[string]struct{}) (int, error)
	DeleteMissingDocumentsUnder(ctx context.Context, folderID int64, root string, seen map[string]struct{}) (int, error)
	DeleteDocumentsForFolder(ctx context.Context, folderPath string) (int, error)
	DeleteDocumentsAtPath(ctx context.Context, folderID int64, path string) (int, error)
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
	if errors.Is(err, os.ErrNotExist) {
		removed, deleteErr := indexer.store.DeleteDocumentsForFolder(ctx, absoluteRoot)
		if deleteErr != nil {
			return Report{}, deleteErr
		}
		return Report{Folder: absoluteRoot, Removed: removed, Errors: []FileError{}}, nil
	}
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

	return indexer.indexTree(ctx, folderID, absoluteRoot, absoluteRoot)
}

func (indexer *Indexer) IndexPath(ctx context.Context, root string, changedPath string) (Report, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return Report{}, fmt.Errorf("resolve folder path: %w", err)
	}
	absoluteRoot = filepath.Clean(absoluteRoot)
	absolutePath, err := filepath.Abs(changedPath)
	if err != nil {
		return Report{}, fmt.Errorf("resolve changed path: %w", err)
	}
	absolutePath = filepath.Clean(absolutePath)
	if !isWithin(absoluteRoot, absolutePath) {
		return Report{}, fmt.Errorf("changed path is outside indexed folder")
	}

	rootInfo, err := os.Stat(absoluteRoot)
	if errors.Is(err, os.ErrNotExist) {
		removed, deleteErr := indexer.store.DeleteDocumentsForFolder(ctx, absoluteRoot)
		if deleteErr != nil {
			return Report{}, deleteErr
		}
		return Report{Folder: absoluteRoot, Removed: removed, Errors: []FileError{}}, nil
	}
	if err != nil {
		return Report{}, fmt.Errorf("read folder: %w", err)
	}
	if !rootInfo.IsDir() {
		return Report{}, fmt.Errorf("index path is not a folder: %s", absoluteRoot)
	}

	folderID, err := indexer.store.RegisterFolder(ctx, absoluteRoot)
	if err != nil {
		return Report{}, err
	}
	info, err := os.Lstat(absolutePath)
	if errors.Is(err, os.ErrNotExist) {
		removed, deleteErr := indexer.store.DeleteDocumentsAtPath(ctx, folderID, absolutePath)
		if deleteErr != nil {
			return Report{}, deleteErr
		}
		return Report{Folder: absoluteRoot, Removed: removed, Errors: []FileError{}}, nil
	}
	if err != nil {
		return Report{}, fmt.Errorf("read changed path: %w", err)
	}
	if info.IsDir() {
		return indexer.indexTree(ctx, folderID, absoluteRoot, absolutePath)
	}
	return indexer.indexFile(ctx, folderID, absoluteRoot, absolutePath, info)
}

func (indexer *Indexer) indexTree(ctx context.Context, folderID int64, root string, treeRoot string) (Report, error) {
	report := Report{Folder: root, Errors: []FileError{}}
	seen := make(map[string]struct{})
	incomplete := false

	err := filepath.WalkDir(treeRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			report.Errors = append(report.Errors, FileError{Path: path, Message: walkErr.Error()})
			incomplete = true
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		fileInfo, err := entry.Info()
		if err != nil {
			report.Errors = append(report.Errors, FileError{Path: path, Message: err.Error()})
			incomplete = true
			return nil
		}
		fileReport, err := indexer.indexFile(ctx, folderID, root, path, fileInfo)
		if err != nil {
			return err
		}
		report.Indexed += fileReport.Indexed
		report.Unchanged += fileReport.Unchanged
		report.Skipped += fileReport.Skipped
		report.Errors = append(report.Errors, fileReport.Errors...)
		if isSupportedPath(path, fileInfo) {
			seen[path] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return Report{}, fmt.Errorf("walk folder: %w", err)
	}

	if !incomplete {
		report.Removed, err = indexer.store.DeleteMissingDocumentsUnder(ctx, folderID, treeRoot, seen)
		if err != nil {
			return Report{}, err
		}
	}
	return report, nil
}

func (indexer *Indexer) indexFile(ctx context.Context, folderID int64, root string, path string, fileInfo fs.FileInfo) (Report, error) {
	report := Report{Folder: root, Errors: []FileError{}}
	if fileInfo.Mode()&os.ModeSymlink != 0 || !isSupportedPath(path, fileInfo) {
		report.Skipped++
		removed, err := indexer.store.DeleteDocumentsAtPath(ctx, folderID, path)
		if err != nil {
			return Report{}, err
		}
		report.Removed = removed
		return report, nil
	}

	current, err := indexer.store.DocumentIsCurrent(ctx, path, fileInfo.Size(), fileInfo.ModTime().UnixNano())
	if err != nil {
		return Report{}, err
	}
	if current {
		if err := indexer.store.EnsureDocumentChunks(ctx, path); err != nil {
			return Report{}, err
		}
		report.Unchanged++
		return report, nil
	}

	extension := strings.ToLower(filepath.Ext(path))
	document := storage.Document{
		FolderID: folderID, Path: path, Name: filepath.Base(path), Extension: extension,
		SizeBytes: fileInfo.Size(), ModifiedAtNS: fileInfo.ModTime().UnixNano(), Status: "indexed",
	}
	document.Content, err = indexer.extractContent(ctx, path, extension, fileInfo.Size())
	if err != nil {
		document.Status = "error"
		document.Error = err.Error()
	}
	if err := indexer.store.UpsertDocument(ctx, document); err != nil {
		return Report{}, err
	}
	if err := indexer.store.EnsureDocumentChunks(ctx, path); err != nil {
		return Report{}, err
	}
	if document.Status == "error" {
		report.Errors = append(report.Errors, FileError{Path: path, Message: document.Error})
	} else {
		report.Indexed++
	}
	return report, nil
}

func isSupportedPath(path string, info fs.FileInfo) bool {
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return false
	}
	extension := strings.ToLower(filepath.Ext(path))
	_, isText := supportedExtensions[extension]
	return isText || extension == pdfExtension
}

func isWithin(root string, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
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
