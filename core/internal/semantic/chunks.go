package semantic

import (
	"strings"
	"unicode/utf8"
)

const DefaultChunkBytes = 1_200
const DefaultOverlapBytes = 200

type Chunk struct {
	Ordinal   int
	StartByte int
	EndByte   int
}

// Split returns overlapping UTF-8-safe passage boundaries. It prefers a
// whitespace break near the target length so semantic results read naturally.
func Split(text string) []Chunk {
	if text == "" {
		return nil
	}
	chunks := make([]Chunk, 0, len(text)/DefaultChunkBytes+1)
	start := 0
	for start < len(text) {
		end := endBoundary(text, start, DefaultChunkBytes)
		if end < len(text) {
			end = preferredBreak(text, start, end)
		}
		chunks = append(chunks, Chunk{Ordinal: len(chunks), StartByte: start, EndByte: end})
		if end == len(text) {
			break
		}
		next := end - DefaultOverlapBytes
		if next <= start {
			next = end
		}
		start = startBoundary(text, next)
	}
	return chunks
}

func endBoundary(text string, start int, length int) int {
	end := start + length
	if end >= len(text) {
		return len(text)
	}
	for end > start && !utf8.RuneStart(text[end]) {
		end--
	}
	return end
}

func startBoundary(text string, start int) int {
	for start < len(text) && !utf8.RuneStart(text[start]) {
		start++
	}
	return start
}

func preferredBreak(text string, start int, end int) int {
	minimum := start + DefaultChunkBytes*3/4
	for position := end; position >= minimum; {
		runeValue, size := utf8.DecodeLastRuneInString(text[start:position])
		if strings.ContainsRune("\n\r\t ", runeValue) {
			return position
		}
		position -= size
	}
	return end
}
