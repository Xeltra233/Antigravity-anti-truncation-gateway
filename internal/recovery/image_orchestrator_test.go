package recovery

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"antigravity-gateway/internal/config"
	"antigravity-gateway/internal/keymgmt"
	"antigravity-gateway/internal/proxy"
)

func TestOrchestrator_TriVariants(t *testing.T) {
	var capturedUpstreamBody []byte
	var capturedUpstreamModel string

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedUpstreamBody = body

		var reqMap map[string]any
		_ = json.Unmarshal(body, &reqMap)
		capturedUpstreamModel, _ = reqMap["model"].(string)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "chatcmpl-test",
			"object": "chat.completion",
			"created": 12345678,
			"model": "gemini-3.7-flash-high",
			"choices": [{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "Hello from mock upstream"
				},
				"finish_reason": "stop"
			}],
			"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
		}`))
	}))
	defer mockUpstream.Close()

	cfg := &config.Config{
		UpstreamBaseURL:             mockUpstream.URL,
		UpstreamAuthMode:            "none",
		UpstreamTimeout:             2 * time.Second,
		UpstreamMaxIdleConns:        10,
		UpstreamMaxIdleConnsPerHost: 10,
		UpstreamMaxConnsPerHost:     10,
		MaxRequestBytes:             1024 * 1024,
		MaxResponseBytes:            1024 * 1024,
		MaxConcurrentRequests:       100,
		MaxConcurrentRequestsPerKey: 100,
		RequestQueueTimeout:         100 * time.Millisecond,
		StandardAliasPrefix:         "[抗截断] ",
		ExperimentalAliasPrefix:     "[实验性] ",
		HybridAliasPrefix:           "[混合实验性] ",
		ImageMaxRunesPerPage:        1500,
		ImageMaxLinesPerPage:        40,
		ImageMaxPages:               100,
		ImageMaxTotalBytes:          12582912,
		ImageMaxSingleBytes:         4194304,
		ImageFallbackOnError:        true,
	}

	keyMgr, _ := keymgmt.NewManager(":memory:", "hmac-secret-at-least-16-bytes", nil)
	client := proxy.NewUpstreamClient(cfg)
	orch := NewOrchestrator(cfg, client, keyMgr)

	// 1. Test [抗截断] gemini-3.7-flash-high
	{
		reqBody := `{"model":"[抗截断] gemini-3.7-flash-high","messages":[{"role":"user","content":"hello world"}]}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(reqBody))
		w := httptest.NewRecorder()

		orch.HandleChatCompletions(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		if capturedUpstreamModel != "gemini-3.7-flash-high" {
			t.Errorf("expected stripped model gemini-3.7-flash-high, got %s", capturedUpstreamModel)
		}
		if !strings.Contains(string(capturedUpstreamBody), "hello world") {
			t.Errorf("expected plain text in upstream body, got: %s", string(capturedUpstreamBody))
		}
	}

	// 2. Test [实验性] gemini-3.7-flash-high
	{
		reqBody := `{"model":"[实验性] gemini-3.7-flash-high","messages":[{"role":"system","content":"sys prompt"},{"role":"user","content":"hello world"}]}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(reqBody))
		w := httptest.NewRecorder()

		orch.HandleChatCompletions(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		if capturedUpstreamModel != "gemini-3.7-flash-high" {
			t.Errorf("expected stripped model gemini-3.7-flash-high, got %s", capturedUpstreamModel)
		}
		if !strings.Contains(string(capturedUpstreamBody), "data:image/png;base64,") {
			t.Errorf("expected image data url in upstream body for [实验性]")
		}
	}

	// 3. Test [混合实验性] gemini-3.7-flash-high
	{
		reqBody := `{"model":"[混合实验性] gemini-3.7-flash-high","messages":[{"role":"system","content":"sys prompt"},{"role":"user","content":"turn 1"},{"role":"assistant","content":"reply 1"},{"role":"user","content":"turn 2"}]}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(reqBody))
		w := httptest.NewRecorder()

		orch.HandleChatCompletions(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		if capturedUpstreamModel != "gemini-3.7-flash-high" {
			t.Errorf("expected stripped model gemini-3.7-flash-high, got %s", capturedUpstreamModel)
		}
		var parsedReq map[string]any
		_ = json.Unmarshal(capturedUpstreamBody, &parsedReq)
		msgs := parsedReq["messages"].([]any)

		// Check that turn 1 user and reply 1 assistant remain text
		m1 := msgs[1].(map[string]any)
		if m1["content"] != "turn 1" {
			t.Errorf("expected turn 1 user to remain string, got %v", m1["content"])
		}
		m2 := msgs[2].(map[string]any)
		if m2["content"] != "reply 1" {
			t.Errorf("expected turn 1 assistant to remain string, got %v", m2["content"])
		}

		// Check that turn 2 (latest user turn) is image
		m3 := msgs[3].(map[string]any)
		parts3 := m3["content"].([]any)
		p0 := parts3[0].(map[string]any)
		if p0["type"] != "image_url" {
			t.Errorf("expected turn 2 to be image_url, got %v", p0["type"])
		}
	}
}
