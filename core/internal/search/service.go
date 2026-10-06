// Package search combines exact FTS matches with local semantic matches.
package search

import (
	"context"
	"sort"

	"github.com/devfiqi/personal-search/core/internal/storage"
)

const reciprocalRankOffset = 60.0

type KeywordSearcher interface {
	Search(ctx context.Context, query string, limit int) ([]storage.SearchResult, error)
}

type SemanticSearcher interface {
	Search(ctx context.Context, query string, limit int) ([]storage.SearchResult, error)
}

type Service struct {
	keyword  KeywordSearcher
	semantic SemanticSearcher
}

func New(keyword KeywordSearcher, semantic SemanticSearcher) *Service {
	return &Service{keyword: keyword, semantic: semantic}
}

// Search uses reciprocal-rank fusion so exact terms and related concepts both
// influence the result order without pretending their raw scores are comparable.
func (service *Service) Search(ctx context.Context, query string, limit int) ([]storage.SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	candidateLimit := limit * 3
	keywords, keywordErr := service.keyword.Search(ctx, query, candidateLimit)
	if keywordErr != nil {
		return nil, keywordErr
	}
	semantic, _ := service.semantic.Search(ctx, query, candidateLimit)

	type rankedResult struct {
		result storage.SearchResult
		score  float64
	}
	merged := make(map[int64]*rankedResult, len(keywords)+len(semantic))
	add := func(results []storage.SearchResult, semanticResult bool) {
		for index, result := range results {
			matchType := "keyword"
			if semanticResult {
				matchType = "semantic"
			}
			item, exists := merged[result.ID]
			if !exists {
				copy := result
				copy.MatchType = matchType
				item = &rankedResult{result: copy}
				merged[result.ID] = item
			} else if item.result.MatchType != matchType {
				item.result.MatchType = "hybrid"
			}
			item.score += 1 / (reciprocalRankOffset + float64(index+1))
			if semanticResult && item.result.Snippet == "" {
				item.result.Snippet = result.Snippet
			}
		}
	}
	add(keywords, false)
	add(semantic, true)

	ranked := make([]rankedResult, 0, len(merged))
	for _, result := range merged {
		result.result.Score = result.score
		ranked = append(ranked, *result)
	}
	sort.SliceStable(ranked, func(left int, right int) bool {
		if ranked[left].score == ranked[right].score {
			return ranked[left].result.ModifiedAtNS > ranked[right].result.ModifiedAtNS
		}
		return ranked[left].score > ranked[right].score
	})
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	results := make([]storage.SearchResult, len(ranked))
	for index, result := range ranked {
		results[index] = result.result
	}
	return results, nil
}
