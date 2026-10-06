package storage

import (
	"context"
	"fmt"
	"strings"
	"unicode"
)

type SearchResult struct {
	ID           int64   `json:"id"`
	Path         string  `json:"path"`
	Name         string  `json:"name"`
	Extension    string  `json:"extension"`
	ModifiedAtNS int64   `json:"modified_at_ns"`
	Snippet      string  `json:"snippet"`
	Score        float64 `json:"score"`
}

func (store *Store) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	parts := parseQuery(query)
	if len(parts) == 0 {
		return []SearchResult{}, nil
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	results, err := store.searchMatch(ctx, buildMatch(parts, " AND "), limit)
	if err != nil {
		return nil, err
	}
	if len(parts) == 1 || len(results) >= limit {
		return results, nil
	}

	partial, err := store.searchMatch(ctx, buildMatch(parts, " OR "), limit)
	if err != nil {
		return nil, err
	}
	seen := make(map[int64]struct{}, len(results))
	for _, result := range results {
		seen[result.ID] = struct{}{}
	}
	for _, result := range partial {
		if len(results) >= limit {
			break
		}
		if _, ok := seen[result.ID]; ok {
			continue
		}
		results = append(results, result)
	}

	return results, nil
}

func (store *Store) searchMatch(ctx context.Context, match string, limit int) ([]SearchResult, error) {
	rows, err := store.database.QueryContext(ctx, `
SELECT
    documents.id,
    documents.path,
    documents.name,
    documents.extension,
	documents.modified_at_ns,
    snippet(documents_fts, 2, '[', ']', '…', 24),
    rank
FROM documents_fts
JOIN documents ON documents.id = documents_fts.rowid
WHERE documents_fts MATCH ?
  AND documents_fts.rank MATCH 'bm25(8.0, 2.0, 1.0)'
  AND documents.status = 'indexed'
ORDER BY rank, documents.modified_at_ns DESC
LIMIT ?`, match, limit)
	if err != nil {
		return nil, fmt.Errorf("search documents: %w", err)
	}
	defer rows.Close()

	results := make([]SearchResult, 0)
	for rows.Next() {
		var result SearchResult
		if err := rows.Scan(
			&result.ID,
			&result.Path,
			&result.Name,
			&result.Extension,
			&result.ModifiedAtNS,
			&result.Snippet,
			&result.Score,
		); err != nil {
			return nil, fmt.Errorf("read search result: %w", err)
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate search results: %w", err)
	}
	return results, nil
}

func parseQuery(query string) []string {
	var parts []string
	var current strings.Builder
	inPhrase := false

	flush := func() {
		value := strings.TrimSpace(current.String())
		current.Reset()
		if value != "" {
			parts = append(parts, value)
		}
	}

	for _, character := range query {
		switch {
		case character == '"':
			flush()
			inPhrase = !inPhrase
		case unicode.IsSpace(character) && !inPhrase:
			flush()
		default:
			current.WriteRune(character)
		}
	}
	flush()

	return parts
}

func buildMatch(parts []string, operator string) string {
	terms := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.ReplaceAll(part, "\"", "\"\"")
		terms = append(terms, "\""+value+"\"")
	}
	return strings.Join(terms, operator)
}
