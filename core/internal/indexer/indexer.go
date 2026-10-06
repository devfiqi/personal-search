package indexer

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/devfiqi/personal-search/core/internal/storage"
)

const MaxTextFileBytes int64 = 10 * 1024 * 1024

var supportedExtensions = map[string]struct{}{
	".c": {}, ".cc": {}, ".cpp": {}, ".css": {}, ".go": {}, ".h": {},
	".hpp": {}, ".html": {}, ".java": {}, ".js": {}, ".json": {},
	".jsx": {}, ".md": {}, ".markdown": {}, ".php": {}, ".py": {},
	".rb": {}, ".rs": {}, ".sh": {}, ".sql": {}, ".swift": {},
	".toml": {}, ".ts": {}, ".tsx": {}, ".txt": {}, ".xml": {},
	".yaml": {}, ".yml": {}, ".zsh": {},
}

type Store interface {
	RegisterFolder(ctx context.Context, path string) (int64, error)
	DocumentIsCurrent(ctx context.Context, path string, sizeBytes int64, modifiedAtNS int64) (bool, error)
	UpsertDocument(ctx context.Context, document storage.Document) error
	DeleteMissingDocuments(ctx context.Context, folderID int64, seen map[string]struct{}) (int, error)
}

type Indexer struct {
	store Store
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

func New(store Store) *Indexer {
	return &Indexer{store: store}
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
		if _, ok := supportedExtensions[extension]; !ok {
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

		if fileInfo.Size() > MaxTextFileBytes {
			document.Status = "error"
			document.Error = "file exceeds text indexing limit"
		} else {
			content, err := os.ReadFile(path)
			switch {
			case err != nil:
				document.Status = "error"
				document.Error = err.Error()
			case !utf8.Valid(content):
				document.Status = "error"
				document.Error = "file is not valid UTF-8"
			default:
				document.Content = string(content)
			}
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
