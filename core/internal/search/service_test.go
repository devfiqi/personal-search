package search

import (
	"context"
	"errors"
	"testing"

	"github.com/devfiqi/personal-search/core/internal/storage"
)

func TestSearchMergesKeywordAndSemanticResults(t *testing.T) {
	service := New(
		staticSearcher{results: []storage.SearchResult{{ID: 1, Name: "exact"}, {ID: 2, Name: "both"}}},
		staticSearcher{results: []storage.SearchResult{{ID: 2, Name: "both"}, {ID: 3, Name: "related"}}},
	)
	results, err := service.Search(context.Background(), "question", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 || results[0].ID != 2 {
		t.Fatalf("Search() = %+v", results)
	}
}

func TestSearchFallsBackWhenSemanticSearchIsUnavailable(t *testing.T) {
	service := New(staticSearcher{results: []storage.SearchResult{{ID: 1}}}, staticSearcher{err: errors.New("model unavailable")})
	results, err := service.Search(context.Background(), "question", 10)
	if err != nil || len(results) != 1 || results[0].ID != 1 {
		t.Fatalf("Search() = %+v, %v", results, err)
	}
}

type staticSearcher struct {
	results []storage.SearchResult
	err     error
}

func (searcher staticSearcher) Search(context.Context, string, int) ([]storage.SearchResult, error) {
	return searcher.results, searcher.err
}
