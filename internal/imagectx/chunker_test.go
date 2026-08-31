package imagectx

import (
	"strings"
	"testing"
)

func TestChunker_Basic(t *testing.T) {
	chunker := NewChunker(100, 10)
	text := "Hello World\nLine 2\nLine 3"
	chunks := chunker.ChunkText(text)

	if len(chunks) == 0 {
		t.Fatalf("expected at least 1 chunk, got 0")
	}

	if chunks[0].TotalCount != len(chunks) {
		t.Errorf("expected TotalCount %d, got %d", len(chunks), chunks[0].TotalCount)
	}

	if !strings.Contains(chunks[0].Text, "Hello World") {
		t.Errorf("expected text to contain Hello World, got %q", chunks[0].Text)
	}
}

func TestChunker_UnicodeCJK(t *testing.T) {
	chunker := NewChunker(20, 5)
	text := "测试中文分片功能。\n第二行中文测试。\n第三行超长内容测试分片是否正确。"
	chunks := chunker.ChunkText(text)

	if len(chunks) < 2 {
		t.Errorf("expected multiple chunks for CJK text, got %d", len(chunks))
	}
}

func TestChunker_Empty(t *testing.T) {
	chunker := NewChunker(100, 10)
	chunks := chunker.ChunkText("")
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty string, got %d", len(chunks))
	}
}
