package server

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"antigravity-gateway/internal/config"
)

func TestServerStartAndShutdown(t *testing.T) {
	cfg := &config.Config{Host: "127.0.0.1", Port: 0, ShutdownTimeout: 2 * time.Second}
	srv := New(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	if err := srv.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}

	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + srv.Addr())
	if err != nil {
		t.Fatalf("request started server: %v", err)
	}
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("response status = %d, want %d", response.StatusCode, http.StatusNoContent)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown server: %v", err)
	}

	probe := &http.Client{Timeout: 200 * time.Millisecond}
	if response, err := probe.Get("http://" + srv.Addr()); err == nil {
		_ = response.Body.Close()
		t.Fatal("server accepted a request after shutdown")
	}
}
