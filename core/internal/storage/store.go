package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	database *sql.DB
}

func Open(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("database path is required")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	dsn := path + separator + "_busy_timeout=5000&_foreign_keys=on&_journal_mode=WAL"

	database, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	database.SetMaxOpenConns(4)
	database.SetMaxIdleConns(4)

	store := &Store{database: database}
	if err := store.migrate(context.Background()); err != nil {
		database.Close()
		return nil, err
	}

	return store, nil
}

func (store *Store) Close() error {
	return store.database.Close()
}

func (store *Store) migrate(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS folders (
    id INTEGER PRIMARY KEY,
    path TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS documents (
    id INTEGER PRIMARY KEY,
    folder_id INTEGER NOT NULL REFERENCES folders(id) ON DELETE CASCADE,
    path TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    extension TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    modified_at_ns INTEGER NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('indexed', 'error')),
    error TEXT,
    indexed_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS documents_folder_id ON documents(folder_id);

CREATE TABLE IF NOT EXISTS semantic_chunks (
    id INTEGER PRIMARY KEY,
    document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    ordinal INTEGER NOT NULL,
    start_byte INTEGER NOT NULL,
    end_byte INTEGER NOT NULL,
    content_hash TEXT NOT NULL,
    vector_state TEXT NOT NULL CHECK (vector_state IN ('pending', 'indexed', 'error')) DEFAULT 'pending',
    vector_error TEXT,
    UNIQUE(document_id, ordinal)
);

CREATE INDEX IF NOT EXISTS semantic_chunks_document_id ON semantic_chunks(document_id);
CREATE INDEX IF NOT EXISTS semantic_chunks_vector_state ON semantic_chunks(vector_state);

CREATE TABLE IF NOT EXISTS gmail_accounts (
    email TEXT PRIMARY KEY,
    client_id TEXT NOT NULL,
    next_page_token TEXT NOT NULL DEFAULT '',
    sync_complete INTEGER NOT NULL DEFAULT 0,
    connected_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS email_messages (
    id INTEGER PRIMARY KEY,
    account_email TEXT NOT NULL REFERENCES gmail_accounts(email) ON DELETE CASCADE,
    gmail_id TEXT NOT NULL,
    thread_id TEXT NOT NULL,
    subject TEXT NOT NULL,
    sender TEXT NOT NULL,
    recipients TEXT NOT NULL,
    received_at_ns INTEGER NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    indexed_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(account_email, gmail_id)
);

CREATE INDEX IF NOT EXISTS email_messages_account_email ON email_messages(account_email);

CREATE VIRTUAL TABLE IF NOT EXISTS email_messages_fts USING fts5(
    subject,
    sender,
    recipients,
    body,
    content='email_messages',
    content_rowid='id',
    tokenize='unicode61 remove_diacritics 2'
);

CREATE TRIGGER IF NOT EXISTS email_messages_after_insert AFTER INSERT ON email_messages BEGIN
    INSERT INTO email_messages_fts(rowid, subject, sender, recipients, body)
    VALUES (new.id, new.subject, new.sender, new.recipients, new.body);
END;

CREATE TRIGGER IF NOT EXISTS email_messages_after_delete AFTER DELETE ON email_messages BEGIN
    INSERT INTO email_messages_fts(email_messages_fts, rowid, subject, sender, recipients, body)
    VALUES ('delete', old.id, old.subject, old.sender, old.recipients, old.body);
END;

CREATE TRIGGER IF NOT EXISTS email_messages_after_update AFTER UPDATE ON email_messages BEGIN
    INSERT INTO email_messages_fts(email_messages_fts, rowid, subject, sender, recipients, body)
    VALUES ('delete', old.id, old.subject, old.sender, old.recipients, old.body);
    INSERT INTO email_messages_fts(rowid, subject, sender, recipients, body)
    VALUES (new.id, new.subject, new.sender, new.recipients, new.body);
END;

CREATE VIRTUAL TABLE IF NOT EXISTS documents_fts USING fts5(
    name,
    path,
    content,
    content='documents',
    content_rowid='id',
    tokenize='unicode61 remove_diacritics 2'
);

CREATE TRIGGER IF NOT EXISTS documents_after_insert AFTER INSERT ON documents BEGIN
    INSERT INTO documents_fts(rowid, name, path, content)
    VALUES (new.id, new.name, new.path, new.content);
END;

CREATE TRIGGER IF NOT EXISTS documents_after_delete AFTER DELETE ON documents BEGIN
    INSERT INTO documents_fts(documents_fts, rowid, name, path, content)
    VALUES ('delete', old.id, old.name, old.path, old.content);
END;

CREATE TRIGGER IF NOT EXISTS documents_after_update AFTER UPDATE ON documents BEGIN
    INSERT INTO documents_fts(documents_fts, rowid, name, path, content)
    VALUES ('delete', old.id, old.name, old.path, old.content);
    INSERT INTO documents_fts(rowid, name, path, content)
    VALUES (new.id, new.name, new.path, new.content);
END;

PRAGMA user_version = 1;
`

	if _, err := store.database.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	if err := store.addColumnIfMissing(ctx, "gmail_accounts", "next_page_token", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := store.addColumnIfMissing(ctx, "gmail_accounts", "sync_complete", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	return nil
}

func (store *Store) addColumnIfMissing(ctx context.Context, table string, column string, definition string) error {
	rows, err := store.database.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return fmt.Errorf("inspect %s schema: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, valueType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &valueType, &notNull, &defaultValue, &primaryKey); err != nil {
			return fmt.Errorf("read %s schema: %w", table, err)
		}
		if name == column {
			return nil
		}
	}
	if _, err := store.database.ExecContext(ctx, "ALTER TABLE "+table+" ADD COLUMN "+column+" "+definition); err != nil {
		return fmt.Errorf("add %s.%s: %w", table, column, err)
	}
	return nil
}
