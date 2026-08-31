package imagectx

import (
	"bytes"
	"image/png"
	"strings"
	"testing"
)

func TestRasterizer_RenderChunk(t *testing.T) {
	cfg := DefaultPipelineConfig()
	rast, err := NewRasterizer(cfg)
	if err != nil {
		t.Fatalf("failed to create rasterizer: %v", err)
	}

	chunk := Chunk{
		Index:      0,
		TotalCount: 1,
		Text:       "Hello World\nLine 2",
		Lines:      []string{"Hello World", "Line 2"},
	}

	img, err := rast.RenderChunk("user", chunk)
	if err != nil {
		t.Fatalf("failed to render chunk: %v", err)
	}

	if img.Width != 1024 {
		t.Errorf("expected width 1024, got %d", img.Width)
	}

	if img.Height <= 0 {
		t.Errorf("expected height > 0, got %d", img.Height)
	}

	if !strings.HasPrefix(img.DataURL, "data:image/png;base64,") {
		t.Errorf("expected valid data URL, got %q", img.DataURL)
	}

	// Verify PNG can be decoded by standard library
	decoded, err := png.Decode(bytes.NewReader(img.PNGBytes))
	if err != nil {
		t.Fatalf("failed to decode rendered PNG bytes: %v", err)
	}
	if decoded.Bounds().Dx() != 1024 {
		t.Errorf("expected decoded width 1024, got %d", decoded.Bounds().Dx())
	}
}

func TestRasterizer_RoleHeader(t *testing.T) {
	cfg := DefaultPipelineConfig()
	rast, err := NewRasterizer(cfg)
	if err != nil {
		t.Fatalf("failed to create rasterizer: %v", err)
	}

	roles := []string{"system", "user", "assistant"}
	for _, r := range roles {
		chunk := Chunk{
			Index:      0,
			TotalCount: 1,
			Text:       "Content for " + r,
			Lines:      []string{"Content for " + r},
		}
		img, err := rast.RenderChunk(r, chunk)
		if err != nil {
			t.Errorf("failed to render for role %s: %v", r, err)
		}
		if img == nil || len(img.PNGBytes) == 0 {
			t.Errorf("empty image bytes for role %s", r)
		}
	}
}
