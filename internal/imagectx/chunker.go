package imagectx

import (
	"strings"
	"unicode/utf8"
)

// Chunk represents a single page of text with line-by-line breakdown.
type Chunk struct {
	Index      int      // 0-indexed page index within message
	TotalCount int      // Total pages in this message
	Text       string   // Full text of this page
	Lines      []string // Lines rendered on this page
	RuneStart  int      // Starting rune index in original text
	RuneEnd    int      // Ending rune index in original text (exclusive)
}

// Chunker handles splitting text into bounded pages.
type Chunker struct {
	MaxRunesPerPage int
	MaxLinesPerPage int
}

// NewChunker creates a new Chunker instance.
func NewChunker(maxRunesPerPage, maxLinesPerPage int) *Chunker {
	if maxRunesPerPage <= 0 {
		maxRunesPerPage = 1500
	}
	if maxLinesPerPage <= 0 {
		maxLinesPerPage = 40
	}
	return &Chunker{
		MaxRunesPerPage: maxRunesPerPage,
		MaxLinesPerPage: maxLinesPerPage,
	}
}

// ChunkText deterministic splits text into chunks.
func (c *Chunker) ChunkText(text string) []Chunk {
	if text == "" {
		return nil
	}

	rawLines := strings.Split(text, "\n")
	var wrappedLines []string

	// Wrap long lines exceeding roughly 90 chars / runes per line for 1024px canvas
	const maxRunesPerLine = 85
	for _, l := range rawLines {
		runes := []rune(l)
		if len(runes) == 0 {
			wrappedLines = append(wrappedLines, "")
			continue
		}
		for len(runes) > maxRunesPerLine {
			wrappedLines = append(wrappedLines, string(runes[:maxRunesPerLine]))
			runes = runes[maxRunesPerLine:]
		}
		if len(runes) > 0 {
			wrappedLines = append(wrappedLines, string(runes))
		}
	}

	var chunks []Chunk
	var currentLines []string
	currentRuneCount := 0
	currentStartRune := 0
	totalRuneOffset := 0

	for _, line := range wrappedLines {
		lineRunes := utf8.RuneCountInString(line)
		if len(currentLines) >= c.MaxLinesPerPage || (currentRuneCount+lineRunes > c.MaxRunesPerPage && len(currentLines) > 0) {
			pageText := strings.Join(currentLines, "\n")
			chunks = append(chunks, Chunk{
				Index:     len(chunks),
				Text:      pageText,
				Lines:     currentLines,
				RuneStart: currentStartRune,
				RuneEnd:   totalRuneOffset,
			})
			currentLines = nil
			currentRuneCount = 0
			currentStartRune = totalRuneOffset
		}

		currentLines = append(currentLines, line)
		currentRuneCount += lineRunes
		totalRuneOffset += lineRunes + 1 // +1 for newline
	}

	if len(currentLines) > 0 {
		pageText := strings.Join(currentLines, "\n")
		chunks = append(chunks, Chunk{
			Index:     len(chunks),
			Text:      pageText,
			Lines:     currentLines,
			RuneStart: currentStartRune,
			RuneEnd:   utf8.RuneCountInString(text),
		})
	}

	total := len(chunks)
	for i := range chunks {
		chunks[i].TotalCount = total
	}

	return chunks
}
