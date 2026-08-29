package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"antigravity-gateway/internal/config"
	"antigravity-gateway/internal/keymgmt"
)

func TestProxyModelsVerbatimAndCache(t *testing.T) {
	upstreamCalled := 0
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled++
		if r.URL.Path != "/v1/models" {
			t.Errorf("expected /v1/models, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-upstream-secret" {
			t.Errorf("unexpected upstream auth: %s", r.Header.Get("Authorization"))
		}

		resp := map[string]any{
			"object": "list",
			"data": []map[string]any{
				{"id": "custom-model-id-verbatim_123", "object": "model"},
				{"id": "gemini-3.5-flash-low", "object": "model"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockUpstream.Close()

	cfg := &config.Config{
		UpstreamBaseURL:             mockUpstream.URL,
		UpstreamAPIKey:              "test-upstream-secret",
		UpstreamAuthMode:            "bearer",
		UpstreamTimeout:             5 * time.Second,
		UpstreamMaxIdleConns:        100,
		UpstreamMaxIdleConnsPerHost: 10,
		UpstreamMaxConnsPerHost:     10,
		ModelsCacheTTL:              500 * time.Millisecond,
		MaxResponseBytes:            1024 * 1024,
	}

	client := NewUpstreamClient(cfg)
	ctx := context.Background()

	// 1. First fetch
	bytes1, err := client.GetModels(ctx, "req1")
	if err != nil {
		t.Fatalf("failed to get models: %v", err)
	}
	if upstreamCalled != 1 {
		t.Errorf("expected 1 upstream call, got %d", upstreamCalled)
	}

	var res1 struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(bytes1, &res1); err != nil {
		t.Fatalf("failed to unmarshal models: %v", err)
	}
	if len(res1.Data) != 2 || res1.Data[0].ID != "custom-model-id-verbatim_123" {
		t.Fatalf("models id not verbatim: %+v", res1)
	}

	// 2. Second fetch within TTL (cached, should not call upstream again)
	bytes2, err := client.GetModels(ctx, "req2")
	if err != nil {
		t.Fatalf("failed to get models from cache: %v", err)
	}
	if upstreamCalled != 1 {
		t.Errorf("expected still 1 upstream call (cached), got %d", upstreamCalled)
	}
	if string(bytes1) != string(bytes2) {
		t.Errorf("cached bytes mismatch")
	}
}

func TestProxyAuthMiddleware(t *testing.T) {
	staticKeys := []config.StaticKeyConfig{
		{
			ID:   "stat-1",
			Key:  "sk-client-key-valid",
			Name: "Valid Key",
		},
	}
	keyMgr, err := keymgmt.NewManager(":memory:", "hmac-secret-at-least-16-bytes", staticKeys)
	if err != nil {
		t.Fatalf("failed to create key manager: %v", err)
	}
	defer keyMgr.Close()

	cfg := &config.Config{
		UpstreamBaseURL: "http://127.0.0.1:9999",
	}
	client := NewUpstreamClient(cfg)
	handler := NewProxyHandler(cfg, keyMgr, client)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Case 1: Missing auth header -> 401
	req := httptest.NewRequest("GET", "/v1/models", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	// Case 2: Invalid auth token -> 401
	req = httptest.NewRequest("GET", "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
