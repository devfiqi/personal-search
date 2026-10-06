package storage

import (
	"context"
	"fmt"
	"net/url"
	"sort"
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
	MatchType    string  `json:"match_type"`
	Source       string  `json:"source"`
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
		result.MatchType = "keyword"
		result.Source = "document"
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate search results: %w", err)
	}
	emails, err := store.searchEmailMatch(ctx, match, limit)
	if err != nil {
		return nil, err
	}
	results = append(results, emails...)
	sort.SliceStable(results, func(left int, right int) bool {
		if results[left].Score == results[right].Score {
			return results[left].ModifiedAtNS > results[right].ModifiedAtNS
		}
		return results[left].Score < results[right].Score
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

const emailResultIDOffset int64 = 1 << 60

func (store *Store) searchEmailMatch(ctx context.Context, match string, limit int) ([]SearchResult, error) {
	rows, err := store.database.QueryContext(ctx, `
SELECT
    email_messages.id,
    email_messages.account_email,
    email_messages.gmail_id,
    email_messages.subject,
    email_messages.received_at_ns,
    snippet(email_messages_fts, 3, '[', ']', '…', 24),
    rank
FROM email_messages_fts
JOIN email_messages ON email_messages.id = email_messages_fts.rowid
WHERE email_messages_fts MATCH ?
  AND email_messages_fts.rank MATCH 'bm25(8.0, 4.0, 2.0, 1.0)'
ORDER BY rank, email_messages.received_at_ns DESC
LIMIT ?`, match, limit)
	if err != nil {
		return nil, fmt.Errorf("search email: %w", err)
	}
	defer rows.Close()
	results := []SearchResult{}
	for rows.Next() {
		var id int64
		var accountEmail, gmailID string
		var result SearchResult
		if err := rows.Scan(&id, &accountEmail, &gmailID, &result.Name, &result.ModifiedAtNS, &result.Snippet, &result.Score); err != nil {
			return nil, fmt.Errorf("read email search result: %w", err)
		}
		result.ID = emailResultIDOffset + id
		result.Path = "https://mail.google.com/mail/u/" + url.PathEscape(accountEmail) + "/#all/" + url.PathEscape(gmailID)
		result.Extension = ".eml"
		result.MatchType = "keyword"
		result.Source = "email"
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate email search results: %w", err)
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
