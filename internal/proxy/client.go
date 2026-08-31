package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"antigravity-gateway/internal/config"
)

type UpstreamClient struct {
	cfg        *config.Config
	httpClient *http.Client

	modelsCacheMu sync.RWMutex
	modelsCache   []byte
	modelsExpiry  time.Time
}

func NewUpstreamClient(cfg *config.Config) *UpstreamClient {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          cfg.UpstreamMaxIdleConns,
		MaxIdleConnsPerHost:   cfg.UpstreamMaxIdleConnsPerHost,
		MaxConnsPerHost:       cfg.UpstreamMaxConnsPerHost,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: cfg.UpstreamTimeout,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   cfg.UpstreamTimeout,
	}

	return &UpstreamClient{
		cfg:        cfg,
		httpClient: httpClient,
	}
}

func (c *UpstreamClient) NewUpstreamRequest(ctx context.Context, method, targetURL string, body io.Reader, reqID string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, targetURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create upstream request: %w", err)
	}

	// Standard content type, accept & user-agent
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")
	// Upstream authentication - NEVER forward downstream auth
	if c.cfg.UpstreamAuthMode == "bearer" && c.cfg.UpstreamAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.UpstreamAPIKey)
	}

	if reqID != "" {
		req.Header.Set("X-Request-ID", reqID)
	}

	return req, nil
}

func (c *UpstreamClient) CloseIdleConnections() {
	c.httpClient.CloseIdleConnections()
}

func (c *UpstreamClient) Do(req *http.Request) (*http.Response, error) {
	return c.httpClient.Do(req)
}

func (c *UpstreamClient) GetModels(ctx context.Context, reqID string) ([]byte, error) {
	c.modelsCacheMu.RLock()
	if c.modelsCache != nil && time.Now().Before(c.modelsExpiry) {
		data := make([]byte, len(c.modelsCache))
		copy(data, c.modelsCache)
		c.modelsCacheMu.RUnlock()
		return data, nil
	}
	c.modelsCacheMu.RUnlock()

	req, err := c.NewUpstreamRequest(ctx, http.MethodGet, c.cfg.UpstreamModelsURL(), nil, reqID)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch models from upstream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("upstream returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, c.cfg.MaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to read models response body: %w", err)
	}

	if c.cfg.ModelsCacheTTL > 0 {
		c.modelsCacheMu.Lock()
		c.modelsCache = bodyBytes
		c.modelsExpiry = time.Now().Add(c.cfg.ModelsCacheTTL)
		c.modelsCacheMu.Unlock()
	}

	return bodyBytes, nil
}
