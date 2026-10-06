package semanticindex

import (
	"context"
	"testing"

	"github.com/devfiqi/personal-search/core/internal/extractor"
	"github.com/devfiqi/personal-search/core/internal/storage"
)

func TestSyncAndSearch(t *testing.T) {
	chunk := storage.SemanticChunk{ID: 9, DocumentID: 4, Path: "/notes/plan.txt", Name: "plan.txt", Extension: ".txt", Text: "Plan a local personal search engine."}
	store := &fakeStore{pending: []storage.SemanticChunk{chunk}, chunks: map[int64]storage.SemanticChunk{chunk.ID: chunk}}
	worker := &fakeWorker{}
	service := New(store, worker)

	indexed, err := service.Sync(context.Background())
	if err != nil || indexed != 1 || len(store.marked) != 1 || store.marked[0] != chunk.ID {
		t.Fatalf("Sync() = %d, %v; marked = %v", indexed, err, store.marked)
	}
	results, err := service.Search(context.Background(), "how do I find my notes?", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ID != chunk.DocumentID || results[0].Snippet != chunk.Text {
		t.Fatalf("Search() = %+v", results)
	}
}

type fakeStore struct {
	pending []storage.SemanticChunk
	chunks  map[int64]storage.SemanticChunk
	marked  []int64
}

func (store *fakeStore) ListPendingSemanticChunks(_ context.Context, _ int) ([]storage.SemanticChunk, error) {
	return store.pending, nil
}

func (store *fakeStore) SemanticChunksByIDs(_ context.Context, ids []int64) (map[int64]storage.SemanticChunk, error) {
	result := make(map[int64]storage.SemanticChunk)
	for _, id := range ids {
		if chunk, ok := store.chunks[id]; ok {
			result[id] = chunk
		}
	}
	return result, nil
}

func (store *fakeStore) MarkSemanticChunksIndexed(_ context.Context, ids []int64) error {
	store.marked = append(store.marked, ids...)
	return nil
}

type fakeWorker struct{}

func (fakeWorker) Embed(_ context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, len(texts))
	for index := range texts {
		vectors[index] = []float32{1, 0}
	}
	return vectors, nil
}

func (fakeWorker) IndexVectors(_ context.Context, _ []int64, _ [][]float32) error { return nil }

func (fakeWorker) SearchVectors(_ context.Context, _ []float32, _ int) ([]extractor.VectorMatch, error) {
	return []extractor.VectorMatch{{ID: 9, Distance: 0.15}}, nil
}
