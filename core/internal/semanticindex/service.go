// Package semanticindex coordinates the local embedding worker, vector index,
// and SQLite passage records.
package semanticindex

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/devfiqi/personal-search/core/internal/extractor"
	"github.com/devfiqi/personal-search/core/internal/storage"
)

const defaultBatchSize = 32

type Store interface {
	ListPendingSemanticChunks(ctx context.Context, limit int) ([]storage.SemanticChunk, error)
	SemanticChunksByIDs(ctx context.Context, ids []int64) (map[int64]storage.SemanticChunk, error)
	MarkSemanticChunksIndexed(ctx context.Context, ids []int64) error
}

type Worker interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	IndexVectors(ctx context.Context, ids []int64, vectors [][]float32) error
	SearchVectors(ctx context.Context, vector []float32, limit int) ([]extractor.VectorMatch, error)
}

type Service struct {
	store  Store
	worker Worker
}

func New(store Store, worker Worker) *Service {
	return &Service{store: store, worker: worker}
}

// Sync indexes one bounded batch so document ingestion and interactive search
// remain responsive while a library is being prepared for semantic retrieval.
func (service *Service) Sync(ctx context.Context) (int, error) {
	chunks, err := service.store.ListPendingSemanticChunks(ctx, defaultBatchSize)
	if err != nil || len(chunks) == 0 {
		return 0, err
	}
	texts := make([]string, len(chunks))
	ids := make([]int64, len(chunks))
	for index, chunk := range chunks {
		texts[index] = chunk.Text
		ids[index] = chunk.ID
	}
	vectors, err := service.worker.Embed(ctx, texts)
	if err != nil {
		return 0, fmt.Errorf("embed semantic passages: %w", err)
	}
	if err := service.worker.IndexVectors(ctx, ids, vectors); err != nil {
		return 0, fmt.Errorf("store semantic vectors: %w", err)
	}
	if err := service.store.MarkSemanticChunksIndexed(ctx, ids); err != nil {
		return 0, err
	}
	return len(chunks), nil
}

// Run incrementally catches up on new and existing passages without holding up
// file watching or keyword indexing. A failed batch remains pending for retry.
func (service *Service) Run(ctx context.Context) {
	for {
		indexed, err := service.Sync(ctx)
		if err != nil && ctx.Err() != nil {
			return
		}
		delay := 750 * time.Millisecond
		if indexed > 0 {
			delay = 50 * time.Millisecond
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (service *Service) Search(ctx context.Context, query string, limit int) ([]storage.SearchResult, error) {
	if query == "" || limit <= 0 {
		return []storage.SearchResult{}, nil
	}
	vectors, err := service.worker.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed semantic query: %w", err)
	}
	if len(vectors) != 1 {
		return nil, fmt.Errorf("semantic worker returned %d query vectors", len(vectors))
	}
	candidateLimit := limit * 5
	if candidateLimit < 50 {
		candidateLimit = 50
	}
	matches, err := service.worker.SearchVectors(ctx, vectors[0], candidateLimit)
	if err != nil || len(matches) == 0 {
		return []storage.SearchResult{}, err
	}
	ids := make([]int64, len(matches))
	for index, match := range matches {
		ids[index] = match.ID
	}
	chunks, err := service.store.SemanticChunksByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	results := make([]storage.SearchResult, 0, limit)
	seenDocuments := make(map[int64]struct{}, limit)
	for _, match := range matches {
		chunk, ok := chunks[match.ID]
		if !ok {
			continue
		}
		if _, seen := seenDocuments[chunk.DocumentID]; seen {
			continue
		}
		seenDocuments[chunk.DocumentID] = struct{}{}
		results = append(results, storage.SearchResult{
			ID: chunk.DocumentID, Path: chunk.Path, Name: chunk.Name, Extension: chunk.Extension,
			ModifiedAtNS: chunk.ModifiedAtNS, Snippet: chunk.Text, Score: 1 - match.Distance, MatchType: "semantic",
		})
		if len(results) == limit {
			break
		}
	}
	sort.SliceStable(results, func(left int, right int) bool { return results[left].Score > results[right].Score })
	return results, nil
}
