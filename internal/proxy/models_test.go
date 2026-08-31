package proxy

import (
	"encoding/json"
	"testing"

	"antigravity-gateway/internal/config"
	"antigravity-gateway/internal/synthetic"
)

func TestFormatTriModelList(t *testing.T) {
	cfg := &config.Config{
		StandardAliasPrefix:     "[抗截断] ",
		ExperimentalAliasPrefix: "[实验性] ",
		HybridAliasPrefix:       "[混合实验性] ",
	}

	filter := synthetic.NewModelFilter("", "")

	rawJSON := `{
		"object": "list",
		"data": [
			{"id": "gemini-3.7-flash-high", "object": "model", "owned_by": "google"},
			{"id": "gemini-3.1-flash-image", "object": "model", "owned_by": "google"},
			{"id": "text-embedding-3-small", "object": "model", "owned_by": "openai"}
		]
	}`

	outBytes, err := FormatTriModelList([]byte(rawJSON), cfg, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var root map[string]any
	if err := json.Unmarshal(outBytes, &root); err != nil {
		t.Fatalf("failed to parse output json: %v", err)
	}

	dataList := root["data"].([]any)
	// Expected:
	// Text model "gemini-3.7-flash-high" -> 3 entries: [抗截断], [实验性], [混合实验性]
	// Image model "gemini-3.1-flash-image" -> 1 entry (raw passthrough)
	// Non-text model "text-embedding-3-small" -> 1 entry (raw passthrough)
	// Total = 3 + 1 + 1 = 5
	if len(dataList) != 5 {
		t.Fatalf("expected 5 models, got %d", len(dataList))
	}

	ids := make([]string, len(dataList))
	for i, itm := range dataList {
		mMap := itm.(map[string]any)
		ids[i] = mMap["id"].(string)
	}

	expectedIDs := []string{
		"[抗截断] gemini-3.7-flash-high",
		"[实验性] gemini-3.7-flash-high",
		"[混合实验性] gemini-3.7-flash-high",
		"gemini-3.1-flash-image",
		"text-embedding-3-small",
	}

	for _, exp := range expectedIDs {
		found := false
		for _, id := range ids {
			if id == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing expected model ID %q in output: %v", exp, ids)
		}
	}
}
