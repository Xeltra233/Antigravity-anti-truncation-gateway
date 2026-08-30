package recovery

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"antigravity-gateway/internal/config"
	"antigravity-gateway/internal/keymgmt"
	"antigravity-gateway/internal/proxy"
)

func TestHighConcurrency500Load(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 500 concurrency load test in short mode")
	}

	var upstreamProcessed atomic.Uint64
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamProcessed.Add(1)
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

		resp := map[string]any{
			"id": "c_bench",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role": "assistant",
						"tool_calls": []map[string]any{
							{
								"id":   "call_bench",
								"type": "function",
								"function": map[string]any{
									"name":      synthToolName,
									"arguments": "{\"content\": \"Concurrency response test.\"}",
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockUpstream.Close()

	cfg := &config.Config{
		UpstreamBaseURL:             mockUpstream.URL,
		UpstreamTimeout:             30 * time.Second,
		UpstreamMaxIdleConns:        5000,
		UpstreamMaxIdleConnsPerHost: 5000,
		UpstreamMaxConnsPerHost:     5000,
		WrapperMode:                 "prefer",
		SyntheticToolPrefix:         "agw_emit_",
		MaxRequestBytes:             1024 * 1024,
		MaxResponseBytes:            1024 * 1024,
		MaxConcurrentRequests:       5000,
		MaxConcurrentRequestsPerKey: 5000,
		RequestQueueTimeout:         1 * time.Second,
	}

	keyMgr, _ := keymgmt.NewManager(":memory:", "hmac-secret-at-least-16-bytes", nil)
	client := proxy.NewUpstreamClient(cfg)
	orch := NewOrchestrator(cfg, client, keyMgr)

	initialGoroutines := runtime.NumGoroutine()
	totalRequests := 5000
	concurrency := 500

	reqCh := make(chan int, totalRequests)
	for i := 0; i < totalRequests; i++ {
		reqCh <- i
	}
	close(reqCh)

	var successCount atomic.Uint64
	var errorCount atomic.Uint64

	start := time.Now()
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for reqIdx := range reqCh {
				body := []byte(`{"model": "gemini-3.5-flash-low", "messages": [{"role": "user", "content": "ping"}]}`)
				req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(body))

				keyInfo := &keymgmt.KeyInfo{ID: "bench_key", Status: "active"}
				ctx := context.WithValue(req.Context(), proxy.KeyInfoContextKey, keyInfo)
				ctx = context.WithValue(ctx, proxy.RequestIDContextKey, "req_bench")
				req = req.WithContext(ctx)

				w := httptest.NewRecorder()
				orch.HandleChatCompletions(w, req)

				if w.Code == http.StatusOK {
					successCount.Add(1)
				} else {
					if errorCount.Add(1) <= 3 {
						t.Logf("Request failed with status %d: %s", w.Code, w.Body.String())
					}
				}
				_ = reqIdx
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	t.Logf("Benchmark finished: %d requests in %v (%.2f req/s), success=%d, errors=%d",
		totalRequests, duration, float64(totalRequests)/duration.Seconds(), successCount.Load(), errorCount.Load())

	if successCount.Load() != uint64(totalRequests) {
		t.Errorf("expected %d successes, got %d (errors: %d)", totalRequests, successCount.Load(), errorCount.Load())
	}

	// Goroutine leak check
	time.Sleep(50 * time.Millisecond)
	finalGoroutines := runtime.NumGoroutine()
	if finalGoroutines-initialGoroutines > 20 {
		t.Errorf("potential goroutine leak: initial=%d, final=%d", initialGoroutines, finalGoroutines)
	}
}
