package keymgmt

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"antigravity-gateway/internal/config"
)

func TestKeyManagerLifecycle(t *testing.T) {
	staticKeys := []config.StaticKeyConfig{
		{
			ID:            "stat-1",
			Key:           "sk-static-test-key",
			Name:          "Static Key 1",
			AllowedModels: []string{"gpt-4o", "gemini-3.5-flash-low"},
		},
	}

	mgr, err := NewManager(":memory:", "hmac-secret-at-least-16-bytes", staticKeys)
	if err != nil {
		t.Fatalf("failed to create key manager: %v", err)
	}
	defer mgr.Close()

	// 1. Test Static Key Auth
	info, err := mgr.Authenticate("sk-static-test-key")
	if err != nil {
		t.Fatalf("failed to auth static key: %v", err)
	}
	if info.ID != "stat-1" || !info.IsStatic {
		t.Errorf("unexpected static key info: %+v", info)
	}
	if !mgr.IsModelAllowed(info, "gpt-4o") {
		t.Errorf("expected gpt-4o to be allowed")
	}
	if mgr.IsModelAllowed(info, "claude-3-5-sonnet") {
		t.Errorf("expected claude-3-5-sonnet to be forbidden")
	}

	// 2. Test Dynamic Key Creation
	res, err := mgr.CreateKey("DynKey1", 0, []string{"all-models"})
	if err != nil {
		t.Fatalf("failed to create dynamic key: %v", err)
	}
	if res.Key == "" || res.ID == "" {
		t.Fatalf("invalid create key result: %+v", res)
	}

	// 3. Test Dynamic Key Auth
	infoDyn, err := mgr.Authenticate(res.Key)
	if err != nil {
		t.Fatalf("failed to auth dynamic key: %v", err)
	}
	if infoDyn.ID != res.ID || infoDyn.Name != "DynKey1" {
		t.Errorf("unexpected dynamic key info: %+v", infoDyn)
	}

	// 4. Test Key Listing (must not contain plaintext key or HMAC hash)
	keys := mgr.ListKeys()
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
	for _, k := range keys {
		if k.HMACHash != "" {
			t.Errorf("HMAC hash leaked in key info: %s", k.ID)
		}
	}

	// 5. Test Revocation
	if err := mgr.RevokeKey(res.ID); err != nil {
		t.Fatalf("failed to revoke key: %v", err)
	}

	// Auth after revoke should fail with ErrKeyRevoked
	_, err = mgr.Authenticate(res.Key)
	if err != ErrKeyRevoked {
		t.Errorf("expected ErrKeyRevoked, got %v", err)
	}

	// Attempting to revoke static key should fail
	err = mgr.RevokeKey("stat-1")
	if err != ErrStaticKey {
		t.Errorf("expected ErrStaticKey, got %v", err)
	}

	// 6. Test Expiration
	expRes, err := mgr.CreateKey("ExpKey", time.Now().Unix()-10, nil) // expired 10s ago
	if err != nil {
		t.Fatalf("failed to create expired key: %v", err)
	}
	_, err = mgr.Authenticate(expRes.Key)
	if err != ErrKeyExpired {
		t.Errorf("expected ErrKeyExpired, got %v", err)
	}
}

func TestAdminAPI(t *testing.T) {
	mgr, err := NewManager(":memory:", "hmac-secret-at-least-16-bytes", nil)
	if err != nil {
		t.Fatalf("failed to create key manager: %v", err)
	}
	defer mgr.Close()

	adminKey := "admin-secret-key-12345"
	adminHandler := NewAdminHandler(adminKey, mgr)
	mux := http.NewServeMux()
	adminHandler.RegisterRoutes(mux)

	// 1. Unauthorized request
	req := httptest.NewRequest("GET", "/admin/keys", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	// 2. Create Key via Admin API
	createBody, _ := json.Marshal(CreateKeyRequest{
		Name:          "API Key 1",
		AllowedModels: []string{"gemini-3.5-flash-low"},
	})
	req = httptest.NewRequest("POST", "/admin/keys", bytes.NewReader(createBody))
	req.Header.Set("Authorization", "Bearer "+adminKey)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
	}

	var created CreateKeyResult
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create key response: %v", err)
	}
	if created.Key == "" || created.ID == "" {
		t.Fatalf("missing key or id in create response: %+v", created)
	}

	// 3. List Keys via Admin API
	req = httptest.NewRequest("GET", "/admin/keys", nil)
	req.Header.Set("Authorization", "Bearer "+adminKey)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
	var listResp struct {
		Data []*KeyInfo `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&listResp); err != nil {
		t.Fatalf("failed to decode list response: %v", err)
	}
	if len(listResp.Data) != 1 || listResp.Data[0].ID != created.ID {
		t.Fatalf("expected 1 key matching ID %s, got %+v", created.ID, listResp.Data)
	}

	// 4. Revoke Key via Admin API
	req = httptest.NewRequest("POST", "/admin/keys/"+created.ID+"/revoke", nil)
	req.Header.Set("Authorization", "Bearer "+adminKey)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	// Verify Auth fails now
	_, err = mgr.Authenticate(created.Key)
	if err != ErrKeyRevoked {
		t.Errorf("expected ErrKeyRevoked, got %v", err)
	}
}

func TestConcurrentAuthAndMutations(t *testing.T) {
	mgr, err := NewManager(":memory:", "hmac-secret-at-least-16-bytes", nil)
	if err != nil {
		t.Fatalf("failed to create key manager: %v", err)
	}
	defer mgr.Close()

	// Pre-create 10 keys
	var keys []string
	for i := 0; i < 10; i++ {
		res, err := mgr.CreateKey("key", 0, nil)
		if err != nil {
			t.Fatalf("create key failed: %v", err)
		}
		keys = append(keys, res.Key)
	}

	var wg sync.WaitGroup
	// 50 goroutines authenticating concurrently
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			k := keys[idx%len(keys)]
			for j := 0; j < 100; j++ {
				_, _ = mgr.Authenticate(k)
			}
		}(i)
	}

	// 5 goroutines creating keys concurrently
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_, _ = mgr.CreateKey("concurrent-key", 0, nil)
			}
		}()
	}

	wg.Wait()
}
