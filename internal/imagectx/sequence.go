package imagectx

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// ItemKind represents whether an item is a native image or rasterized text.
type ItemKind string

const (
	KindNativeImage ItemKind = "native_image"
	KindTextImage   ItemKind = "text_image"
	KindNativeText  ItemKind = "native_text"
)

// NormalizedItem represents a sequenced part of a message.
type NormalizedItem struct {
	MessageIndex int      `json:"message_index"`
	Role         string   `json:"role"`
	ContentIndex int      `json:"content_index"`
	PartIndex    int      `json:"part_index"`
	PartCount    int      `json:"part_count"`
	Kind         ItemKind `json:"kind"`
	MIME         string   `json:"mime"`
	ByteLength   int64    `json:"byte_length"`
	SHA256       string   `json:"sha256"`
	SourceText   string   `json:"source_text,omitempty"`
	DataURL      string   `json:"data_url,omitempty"`
}

// NormalizedSequence is an ordered collection of NormalizedItems.
type NormalizedSequence struct {
	Items      []NormalizedItem `json:"items"`
	TotalBytes int64            `json:"total_bytes"`
	TotalPages int              `json:"total_pages"`
}

// Validate checks the sequence for ordering consistency and duplicate parts.
func (s *NormalizedSequence) Validate() error {
	seenKeys := make(map[string]bool)
	for idx, itm := range s.Items {
		if itm.PartIndex < 0 || (itm.PartCount > 0 && itm.PartIndex >= itm.PartCount) {
			return fmt.Errorf("item at index %d has invalid part index %d/%d", idx, itm.PartIndex, itm.PartCount)
		}
		key := fmt.Sprintf("%d:%d:%d", itm.MessageIndex, itm.ContentIndex, itm.PartIndex)
		if seenKeys[key] {
			return fmt.Errorf("duplicate item key %s in sequence", key)
		}
		seenKeys[key] = true
	}
	return nil
}

// ComputeHash computes a SHA256 over the concatenated item hashes.
func (s *NormalizedSequence) ComputeHash() string {
	h := sha256.New()
	for _, itm := range s.Items {
		h.Write([]byte(itm.SHA256))
		h.Write([]byte(itm.Role))
		h.Write([]byte(itm.Kind))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ReconstructText joins all text items in sequence.
func (s *NormalizedSequence) ReconstructText() string {
	var sb strings.Builder
	for _, itm := range s.Items {
		if itm.SourceText != "" {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(itm.SourceText)
		}
	}
	return sb.String()
}
