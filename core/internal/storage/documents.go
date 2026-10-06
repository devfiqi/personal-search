package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

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
