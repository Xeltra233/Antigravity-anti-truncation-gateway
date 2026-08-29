package recovery

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"antigravity-gateway/internal/config"
	"antigravity-gateway/internal/keymgmt"
	"antigravity-gateway/internal/proxy"
)

func TestEmptyResponseRetrySuccess(t *testing.T) {
	var callCount atomic.Int32
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := callCount.Add(1)
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)

		// Extract synthetic tool name
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

		if count <= 2 {
			// First 2 calls return empty response (空回)
			resp := map[string]any{
				"id": "c_empty",
				"choices": []map[string]any{
					{
						"index": 0,
						"message": map[string]any{
							"role":       "assistant",
							"content":    nil,
							"tool_calls": nil,
						},
						"finish_reason": "stop",
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		} else {
			// 3rd call returns actual answer
			resp := map[string]any{
				"id": "c_success",
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
										"arguments": "{\"content\": \"Success after 2 empty retries!\"}",
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
		UpstreamBaseURL:             mockUpstream.URL,
		UpstreamTimeout:             5 * time.Second,
		WrapperMode:                 "prefer",
		UpstreamEmptyRetries:        3,
		SyntheticToolPrefix:         "agw_emit_",
		MaxRequestBytes:             1024 * 1024,
		MaxResponseBytes:            1024 * 1024,
		MaxConcurrentRequests:       100,
		MaxConcurrentRequestsPerKey: 10,
		RequestQueueTimeout:         50 * time.Millisecond,
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

	keyInfo := &keymgmt.KeyInfo{ID: "key_empty_test", Status: "active"}
	ctx := context.WithValue(req.Context(), proxy.KeyInfoContextKey, keyInfo)
	ctx = context.WithValue(ctx, proxy.RequestIDContextKey, "req_empty_test")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	orch.HandleChatCompletions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	if callCount.Load() != 3 {
		t.Errorf("expected exactly 3 upstream calls (2 empty retries + 1 success), got %d", callCount.Load())
	}

	var resp map[string]any
	_ = json.NewDecoder(w.Body).Decode(&resp)
	choices := resp["choices"].([]any)
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	if msg["content"] != "Success after 2 empty retries!" {
		t.Errorf("content mismatch: %v", msg["content"])
	}
}

func TestConcurrencyLimiterRejections(t *testing.T) {
	// Mock slow upstream to saturate limiter
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		resp := map[string]any{
			"choices": []map[string]any{
				{"index": 0, "message": map[string]any{"role": "assistant", "content": "ok"}},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockUpstream.Close()

	cfg := &config.Config{
		UpstreamBaseURL:             mockUpstream.URL,
		UpstreamTimeout:             5 * time.Second,
		WrapperMode:                 "off",
		MaxRequestBytes:             1024 * 1024,
		MaxResponseBytes:            1024 * 1024,
		MaxConcurrentRequests:       2,
		MaxConcurrentRequestsPerKey: 1,
		RequestQueueTimeout:         10 * time.Millisecond,
	}

	keyMgr, _ := keymgmt.NewManager(":memory:", "hmac-secret-at-least-16-bytes", nil)
	client := proxy.NewUpstreamClient(cfg)
	orch := NewOrchestrator(cfg, client, keyMgr)

	var wg sync.WaitGroup
	statusCodes := make([]int, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"gpt-4o","messages":[]}`)))
			keyInfo := &keymgmt.KeyInfo{ID: "shared_key", Status: "active"}
			ctx := context.WithValue(req.Context(), proxy.KeyInfoContextKey, keyInfo)
			ctx = context.WithValue(ctx, proxy.RequestIDContextKey, "req_limit")
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()
			orch.HandleChatCompletions(w, req)
			statusCodes[idx] = w.Code
		}(i)
	}

	wg.Wait()

	// Verify some requests received 429 Too Many Requests
	has429 := false
	has200 := false
	for _, code := range statusCodes {
		if code == http.StatusTooManyRequests {
			has429 = true
		}
		if code == http.StatusOK {
			has200 = true
		}
	}

	if !has429 {
		t.Errorf("expected at least one 429 response when per-key limit was 1, got codes: %v", statusCodes)
	}
	if !has200 {
		t.Errorf("expected at least one 200 response, got codes: %v", statusCodes)
	}
}
