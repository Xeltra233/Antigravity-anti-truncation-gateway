package recovery

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"antigravity-gateway/internal/config"
	"antigravity-gateway/internal/keymgmt"
	"antigravity-gateway/internal/proxy"
)

func TestOrchestratorNonStreamingAndRetry(t *testing.T) {
	var callCount atomic.Int32
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := callCount.Add(1)
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)

		// Find synthetic tool name from tools
		tools, _ := req["tools"].([]any)
		var synthToolName string
		for _, t := range tools {
			tMap := t.(map[string]any)
			fn := tMap["function"].(map[string]any)
			name := fn["name"].(string)
			if strings.HasPrefix(name, "agw_emit_") {
				synthToolName = name
				break
			}
		}

		if count == 1 {
			// First call returns completely unparseable arguments
			resp := map[string]any{
				"id": "c1",
				"choices": []map[string]any{
					{
						"index": 0,
						"message": map[string]any{
							"role": "assistant",
							"tool_calls": []map[string]any{
								{
									"id":   "call_1",
									"type": "function",
									"function": map[string]any{
										"name":      synthToolName,
										"arguments": "NOT_JSON_AT_ALL",
									},
								},
							},
						},
						"finish_reason": "tool_calls",
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		} else {
			// Second retry call returns valid arguments
			resp := map[string]any{
				"id": "c2",
				"choices": []map[string]any{
					{
						"index": 0,
						"message": map[string]any{
							"role": "assistant",
							"tool_calls": []map[string]any{
								{
									"id":   "call_2",
									"type": "function",
									"function": map[string]any{
										"name":      synthToolName,
										"arguments": "{\"content\": \"Recovered answer on retry.\"}",
									},
								},
							},
						},
						"finish_reason": "tool_calls",
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}
	}))
	defer mockUpstream.Close()

	cfg := &config.Config{
		UpstreamBaseURL:     mockUpstream.URL,
		UpstreamTimeout:     5 * time.Second,
		WrapperMode:         "prefer",
		RecoveryPolicy:      "repair_then_retry",
		SyntheticToolPrefix: "agw_emit_",
		MaxRequestBytes:     1024 * 1024,
		MaxResponseBytes:    1024 * 1024,
	}

	keyMgr, err := keymgmt.NewManager(":memory:", "hmac-secret-at-least-16-bytes", nil)
	if err != nil {
		t.Fatalf("failed to create key manager: %v", err)
	}
	defer keyMgr.Close()

	client := proxy.NewUpstreamClient(cfg)
	orch := NewOrchestrator(cfg, client, keyMgr)

	reqBody := []byte(`{"model": "gpt-4o", "messages": [{"role": "user", "content": "hi"}]}`)
	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(reqBody))

	// Inject key info into context
	keyInfo := &keymgmt.KeyInfo{ID: "k1", Status: "active"}
	ctx := context.WithValue(req.Context(), proxy.KeyInfoContextKey, keyInfo)
	ctx = context.WithValue(ctx, proxy.RequestIDContextKey, "req_test_123")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	orch.HandleChatCompletions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK after retry, got %d: %s", w.Code, w.Body.String())
	}
	if callCount.Load() != 2 {
		t.Errorf("expected 2 upstream calls (1 initial + 1 retry), got %d", callCount.Load())
	}

	var resp map[string]any
	_ = json.NewDecoder(w.Body).Decode(&resp)
	choices := resp["choices"].([]any)
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	if msg["content"] != "Recovered answer on retry." {
		t.Errorf("content mismatch: %v", msg["content"])
	}
}
