package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type Folder struct {
	ID   int64  `json:"id"`
	Path string `json:"path"`
}

type Failure struct {
	ID        int64  `json:"id"`
	Path      string `json:"path"`
	Name      string `json:"name"`
	Extension string `json:"extension"`
	Message   string `json:"message"`
}

type Document struct {
	FolderID     int64
	Path         string
	Name         string
	Extension    string
	SizeBytes    int64
	ModifiedAtNS int64
	Content      string
	Status       string
	Error        string
}

func (store *Store) RegisterFolder(ctx context.Context, path string) (int64, error) {
	if _, err := store.database.ExecContext(ctx, `
INSERT INTO folders(path) VALUES (?)
ON CONFLICT(path) DO NOTHING`, path); err != nil {
		return 0, fmt.Errorf("register folder: %w", err)
	}

	var id int64
	if err := store.database.QueryRowContext(ctx, "SELECT id FROM folders WHERE path = ?", path).Scan(&id); err != nil {
		return 0, fmt.Errorf("read folder: %w", err)
	}
	return id, nil
}

func (store *Store) ListFolders(ctx context.Context) ([]Folder, error) {
	rows, err := store.database.QueryContext(ctx, "SELECT id, path FROM folders ORDER BY path")
	if err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}
	defer rows.Close()

	folders := []Folder{}
	for rows.Next() {
		var folder Folder
		if err := rows.Scan(&folder.ID, &folder.Path); err != nil {
			return nil, fmt.Errorf("read folder: %w", err)
		}
		folders = append(folders, folder)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate folders: %w", err)
	}
	return folders, nil
}

func (store *Store) RemoveFolder(ctx context.Context, path string) (bool, error) {
	result, err := store.database.ExecContext(ctx, "DELETE FROM folders WHERE path = ?", path)
	if err != nil {
		return false, fmt.Errorf("remove folder: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read removed folder count: %w", err)
	}
	return count > 0, nil
}

func (store *Store) Reset(ctx context.Context) error {
	if _, err := store.database.ExecContext(ctx, "DELETE FROM folders"); err != nil {
		return fmt.Errorf("reset local index: %w", err)
	}
	return nil
}

func (store *Store) ListFailures(ctx context.Context, limit int) ([]Failure, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	rows, err := store.database.QueryContext(ctx, `
SELECT id, path, name, extension, COALESCE(error, '')
FROM documents
WHERE status = 'error'
ORDER BY indexed_at DESC, id DESC
LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list extraction failures: %w", err)
	}
	defer rows.Close()

	failures := []Failure{}
	for rows.Next() {
		var failure Failure
		if err := rows.Scan(&failure.ID, &failure.Path, &failure.Name, &failure.Extension, &failure.Message); err != nil {
			return nil, fmt.Errorf("read extraction failure: %w", err)
		}
		failures = append(failures, failure)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate extraction failures: %w", err)
	}
	return failures, nil
}

func (store *Store) DocumentIsCurrent(ctx context.Context, path string, sizeBytes int64, modifiedAtNS int64) (bool, error) {
	var storedSize int64
	var storedModified int64
	var status string
	err := store.database.QueryRowContext(ctx, `
SELECT size_bytes, modified_at_ns, status FROM documents WHERE path = ?`, path).Scan(&storedSize, &storedModified, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read document fingerprint: %w", err)
	}

	return storedSize == sizeBytes && storedModified == modifiedAtNS && status == "indexed", nil
}

func (store *Store) UpsertDocument(ctx context.Context, document Document) error {
	_, err := store.database.ExecContext(ctx, `
INSERT INTO documents(
    folder_id, path, name, extension, size_bytes, modified_at_ns, content, status, error
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''))
ON CONFLICT(path) DO UPDATE SET
    folder_id = excluded.folder_id,
    name = excluded.name,
    extension = excluded.extension,
    size_bytes = excluded.size_bytes,
    modified_at_ns = excluded.modified_at_ns,
    content = excluded.content,
    status = excluded.status,
    error = excluded.error,
    indexed_at = CURRENT_TIMESTAMP`,
		document.FolderID,
		document.Path,
		document.Name,
		document.Extension,
		document.SizeBytes,
		document.ModifiedAtNS,
		document.Content,
		document.Status,
		document.Error,
	)
	if err != nil {
		return fmt.Errorf("upsert document: %w", err)
	}
	return nil
}

func (store *Store) DeleteMissingDocuments(ctx context.Context, folderID int64, seen map[string]struct{}) (int, error) {
	rows, err := store.database.QueryContext(ctx, "SELECT path FROM documents WHERE folder_id = ?", folderID)
	if err != nil {
		return 0, fmt.Errorf("list indexed documents: %w", err)
	}

	var missing []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			rows.Close()
			return 0, fmt.Errorf("read indexed document path: %w", err)
		}
		if _, ok := seen[path]; !ok {
			missing = append(missing, path)
		}
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close indexed document rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate indexed documents: %w", err)
	}

	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin document cleanup: %w", err)
	}
	defer transaction.Rollback()

	for _, path := range missing {
		if _, err := transaction.ExecContext(ctx, "DELETE FROM documents WHERE path = ?", path); err != nil {
			return 0, fmt.Errorf("delete missing document: %w", err)
		}
	}

	if err := transaction.Commit(); err != nil {
		return 0, fmt.Errorf("commit document cleanup: %w", err)
	}
	return len(missing), nil
}
