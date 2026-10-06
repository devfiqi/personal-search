package semantic

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSplitProducesOverlappingUTF8SafePassages(t *testing.T) {
	text := strings.Repeat("multilingual résumé مرحبا ", 160)
	chunks := Split(text)
	if len(chunks) < 2 {
		t.Fatalf("Split() returned %d chunks, want more than one", len(chunks))
	}
	for index, chunk := range chunks {
		if chunk.Ordinal != index || chunk.StartByte >= chunk.EndByte {
			t.Fatalf("chunk %d = %+v", index, chunk)
		}
		if !utf8.ValidString(text[chunk.StartByte:chunk.EndByte]) {
			t.Fatalf("chunk %d splits a UTF-8 rune", index)
		}
		if index > 0 && chunk.StartByte >= chunks[index-1].EndByte {
			t.Fatalf("chunk %d does not overlap the previous chunk", index)
		}
	}
	if chunks[len(chunks)-1].EndByte != len(text) {
		t.Fatalf("last chunk ends at %d, want %d", chunks[len(chunks)-1].EndByte, len(text))
	}
}

func TestSplitEmptyText(t *testing.T) {
	if chunks := Split(""); len(chunks) != 0 {
		t.Fatalf("Split() returned %d chunks for empty text", len(chunks))
	}
}
