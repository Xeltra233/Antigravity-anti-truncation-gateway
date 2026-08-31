package imagectx

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// RenderedImage represents the output of a rasterized text page.
type RenderedImage struct {
	PageNumber int    `json:"page_number"`
	TotalPages int    `json:"total_pages"`
	Role       string `json:"role"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	ByteLength int64  `json:"byte_length"`
	SHA256     string `json:"sha256"`
	PNGBytes   []byte `json:"-"`
	DataURL    string `json:"data_url"`
}

// Rasterizer renders text chunks into PNG images.
type Rasterizer struct {
	cfg        *PipelineConfig
	fontFace   font.Face
	width      int
	lineHeight int
	paddingX   int
	paddingY   int
	headerH    int
}

// NewRasterizer creates a new Rasterizer instance with the given configuration.
func NewRasterizer(cfg *PipelineConfig) (*Rasterizer, error) {
	if cfg == nil {
		cfg = DefaultPipelineConfig()
	}

	face, err := GetDefaultFontFace(cfg.FontPath, 16.0)
	if err != nil {
		return nil, fmt.Errorf("failed to load font face: %w", err)
	}

	return &Rasterizer{
		cfg:        cfg,
		fontFace:   face,
		width:      1024,
		lineHeight: 24,
		paddingX:   28,
		paddingY:   20,
		headerH:    42,
	}, nil
}

// RenderChunk renders a single Chunk with the specified role into a RenderedImage.
func (r *Rasterizer) RenderChunk(role string, chunk Chunk) (*RenderedImage, error) {
	numLines := len(chunk.Lines)
	if numLines == 0 {
		numLines = 1
	}

	calculatedHeight := r.headerH + r.paddingY*2 + (numLines * r.lineHeight) + 16
	if calculatedHeight < 120 {
		calculatedHeight = 120
	}

	img := image.NewRGBA(image.Rect(0, 0, r.width, calculatedHeight))

	// Background: Off-white #F8F9FA
	bgColor := color.RGBA{R: 248, G: 249, B: 250, A: 255}
	draw.Draw(img, img.Bounds(), &image.Uniform{C: bgColor}, image.Point{}, draw.Src)

	// Header background banner: #E9ECEF
	headerRect := image.Rect(0, 0, r.width, r.headerH)
	headerBgColor := color.RGBA{R: 233, G: 236, B: 239, A: 255}
	draw.Draw(img, headerRect, &image.Uniform{C: headerBgColor}, image.Point{}, draw.Src)

	// Header divider line: #CED4DA
	dividerRect := image.Rect(0, r.headerH-1, r.width, r.headerH)
	dividerColor := color.RGBA{R: 206, G: 212, B: 218, A: 255}
	draw.Draw(img, dividerRect, &image.Uniform{C: dividerColor}, image.Point{}, draw.Src)

	// Header text: [Role: User] [Part 1/2]
	roleDisplayName := strings.ToUpper(role)
	if roleDisplayName == "" {
		roleDisplayName = "USER"
	}
	headerTitle := fmt.Sprintf("[Role: %s]  [Part %d/%d]", roleDisplayName, chunk.Index+1, chunk.TotalCount)

	headerDrawer := &font.Drawer{
		Dst:  img,
		Src:  &image.Uniform{C: color.RGBA{R: 73, G: 80, B: 87, A: 255}}, // Dark slate gray #495057
		Face: r.fontFace,
		Dot:  fixed.Point26_6{X: fixed.I(r.paddingX), Y: fixed.I(r.headerH - 14)},
	}
	headerDrawer.DrawString(headerTitle)

	// Content text drawer: #212529 (almost black)
	textColor := color.RGBA{R: 33, G: 37, B: 41, A: 255}
	contentDrawer := &font.Drawer{
		Dst:  img,
		Src:  &image.Uniform{C: textColor},
		Face: r.fontFace,
	}

	startY := r.headerH + r.paddingY + 16
	for i, line := range chunk.Lines {
		yPos := startY + (i * r.lineHeight)
		contentDrawer.Dot = fixed.Point26_6{
			X: fixed.I(r.paddingX),
			Y: fixed.I(yPos),
		}
		contentDrawer.DrawString(line)
	}

	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("png encoding failed: %w", err)
	}

	pngBytes := buf.Bytes()
	byteLen := int64(len(pngBytes))

	if r.cfg != nil && r.cfg.MaxSingleBytes > 0 && byteLen > r.cfg.MaxSingleBytes {
		return nil, fmt.Errorf("rendered png size (%d bytes) exceeds single image limit (%d bytes)", byteLen, r.cfg.MaxSingleBytes)
	}

	hash := sha256.Sum256(pngBytes)
	hashHex := hex.EncodeToString(hash[:])
	b64 := base64.StdEncoding.EncodeToString(pngBytes)
	dataURL := "data:image/png;base64," + b64

	return &RenderedImage{
		PageNumber: chunk.Index + 1,
		TotalPages: chunk.TotalCount,
		Role:       role,
		Width:      r.width,
		Height:     calculatedHeight,
		ByteLength: byteLen,
		SHA256:     hashHex,
		PNGBytes:   pngBytes,
		DataURL:    dataURL,
	}, nil
}
