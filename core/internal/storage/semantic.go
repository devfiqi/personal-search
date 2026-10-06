package storage

import (
	"context"
	"fmt"
	"strings"
)

// SemanticChunk is a passage awaiting or available for vector retrieval. Text
// is read from the canonical document content using its stored byte offsets.
type SemanticChunk struct {
	ID           int64
	DocumentID   int64
	Path         string
	Name         string
	Extension    string
	ModifiedAtNS int64
	Text         string
}

func (store *Store) PendingSemanticChunkCount(ctx context.Context) (int, error) {
	var count int
	if err := store.database.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM semantic_chunks
JOIN documents ON documents.id = semantic_chunks.document_id
WHERE semantic_chunks.vector_state = 'pending' AND documents.status = 'indexed'`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count pending semantic chunks: %w", err)
	}
	return count, nil
}

func (store *Store) ListPendingSemanticChunks(ctx context.Context, limit int) ([]SemanticChunk, error) {
	if limit <= 0 {
		limit = 32
	}
	if limit > 128 {
		limit = 128
	}
	return store.listSemanticChunks(ctx, `
WHERE semantic_chunks.vector_state = 'pending'
  AND documents.status = 'indexed'
ORDER BY semantic_chunks.id
LIMIT ?`, limit)
}

func (store *Store) SemanticChunksByIDs(ctx context.Context, ids []int64) (map[int64]SemanticChunk, error) {
	if len(ids) == 0 {
		return map[int64]SemanticChunk{}, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	arguments := make([]any, len(ids))
	for index, id := range ids {
		arguments[index] = id
	}
	chunks, err := store.listSemanticChunks(ctx, `
WHERE semantic_chunks.id IN (`+placeholders+`)
  AND semantic_chunks.vector_state = 'indexed'
  AND documents.status = 'indexed'`, arguments...)
	if err != nil {
		return nil, err
	}
	result := make(map[int64]SemanticChunk, len(chunks))
	for _, chunk := range chunks {
		result[chunk.ID] = chunk
	}
	return result, nil
}

func (store *Store) MarkSemanticChunksIndexed(ctx context.Context, ids []int64) error {
	return store.updateSemanticChunkState(ctx, ids, "indexed", "")
}

func (store *Store) MarkSemanticChunksError(ctx context.Context, ids []int64, message string) error {
	return store.updateSemanticChunkState(ctx, ids, "error", message)
}

func (store *Store) listSemanticChunks(ctx context.Context, condition string, arguments ...any) ([]SemanticChunk, error) {
	rows, err := store.database.QueryContext(ctx, `
SELECT
    semantic_chunks.id,
    documents.id,
    documents.path,
    documents.name,
    documents.extension,
    documents.modified_at_ns,
    documents.content,
    semantic_chunks.start_byte,
    semantic_chunks.end_byte
FROM semantic_chunks
JOIN documents ON documents.id = semantic_chunks.document_id
`+condition, arguments...)
	if err != nil {
		return nil, fmt.Errorf("list semantic chunks: %w", err)
	}
	defer rows.Close()

	chunks := []SemanticChunk{}
	for rows.Next() {
		var chunk SemanticChunk
		var content string
		var startByte, endByte int
		if err := rows.Scan(
			&chunk.ID,
			&chunk.DocumentID,
			&chunk.Path,
			&chunk.Name,
			&chunk.Extension,
			&chunk.ModifiedAtNS,
			&content,
			&startByte,
			&endByte,
		); err != nil {
			return nil, fmt.Errorf("read semantic chunk: %w", err)
		}
		if startByte < 0 || endByte <= startByte || endByte > len(content) {
			return nil, fmt.Errorf("semantic chunk %d has invalid text offsets", chunk.ID)
		}
		chunk.Text = content[startByte:endByte]
		chunks = append(chunks, chunk)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate semantic chunks: %w", err)
	}
	return chunks, nil
}

func (store *Store) updateSemanticChunkState(ctx context.Context, ids []int64, state string, message string) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	arguments := make([]any, 0, len(ids)+2)
	arguments = append(arguments, state, message)
	for _, id := range ids {
		arguments = append(arguments, id)
	}
	if _, err := store.database.ExecContext(ctx, `
UPDATE semantic_chunks
SET vector_state = ?, vector_error = NULLIF(?, '')
WHERE id IN (`+placeholders+`)`, arguments...); err != nil {
		return fmt.Errorf("update semantic chunk state: %w", err)
	}
	return nil
}
