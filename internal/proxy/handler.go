package proxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"

	"antigravity-gateway/internal/config"
	"antigravity-gateway/internal/keymgmt"
	"antigravity-gateway/internal/server"
)

type contextKey string

const (
	KeyInfoContextKey   contextKey = "key_info"
	RequestIDContextKey contextKey = "request_id"
)

func GenerateRequestID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "req_" + hex.EncodeToString(b)
}

func GetKeyInfo(ctx context.Context) *keymgmt.KeyInfo {
	val := ctx.Value(KeyInfoContextKey)
	if val == nil {
		return nil
	}
	return val.(*keymgmt.KeyInfo)
}

func GetRequestID(ctx context.Context) string {
	val := ctx.Value(RequestIDContextKey)
	if val == nil {
		return ""
	}
	return val.(string)
}

type ProxyHandler struct {
	cfg         *config.Config
	keyMgr      *keymgmt.Manager
	client      *UpstreamClient
	chatHandler http.HandlerFunc
}

func NewProxyHandler(cfg *config.Config, keyMgr *keymgmt.Manager, client *UpstreamClient) *ProxyHandler {
	return &ProxyHandler{
		cfg:    cfg,
		keyMgr: keyMgr,
		client: client,
	}
}

func (h *ProxyHandler) SetChatHandler(fn http.HandlerFunc) {
	h.chatHandler = fn
}

func (h *ProxyHandler) DownstreamAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			reqID = GenerateRequestID()
		}
		w.Header().Set("X-Request-ID", reqID)

		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			server.WriteError(w, http.StatusUnauthorized, "Missing or invalid Authorization header", "auth_error", "invalid_api_key")
			return
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")

		info, err := h.keyMgr.Authenticate(token)
		if err != nil {
			if errors.Is(err, keymgmt.ErrKeyRevoked) {
				server.WriteError(w, http.StatusUnauthorized, "API key has been revoked", "auth_error", "api_key_revoked")
				return
			}
			if errors.Is(err, keymgmt.ErrKeyExpired) {
				server.WriteError(w, http.StatusUnauthorized, "API key has expired", "auth_error", "api_key_expired")
				return
			}
			server.WriteError(w, http.StatusUnauthorized, "Incorrect API key provided", "auth_error", "invalid_api_key")
			return
		}

		ctx := context.WithValue(r.Context(), KeyInfoContextKey, info)
		ctx = context.WithValue(ctx, RequestIDContextKey, reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *ProxyHandler) HandleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		server.WriteError(w, http.StatusMethodNotAllowed, "Method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}

	reqID := GetRequestID(r.Context())
	modelsBytes, err := h.client.GetModels(r.Context(), reqID)
	if err != nil {
		server.WriteError(w, http.StatusBadGateway, "Failed to get models from upstream: "+err.Error(), "api_error", "upstream_error")
		return
	}

	formattedBytes, err := FormatTriModelList(modelsBytes, h.cfg, nil)
	if err != nil {
		formattedBytes = modelsBytes
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(formattedBytes)
}

func (h *ProxyHandler) HandleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.WriteError(w, http.StatusMethodNotAllowed, "Method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}

	if h.chatHandler != nil {
		h.chatHandler(w, r)
		return
	}

	server.WriteError(w, http.StatusNotImplemented, "Chat completions handler not configured", "api_error", "not_implemented")
}

// HandleGenericPassthrough proxies any other /v1/... endpoint (e.g. /v1/embeddings, /v1/completions) to upstream.
func (h *ProxyHandler) HandleGenericPassthrough(w http.ResponseWriter, r *http.Request) {
	reqID := GetRequestID(r.Context())
	targetURL := h.cfg.UpstreamBaseURL + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	upstreamReq, err := h.client.NewUpstreamRequest(r.Context(), r.Method, targetURL, r.Body, reqID)
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "Failed to create upstream request: "+err.Error(), "api_error", "internal_error")
		return
	}

	if ct := r.Header.Get("Content-Type"); ct != "" {
		upstreamReq.Header.Set("Content-Type", ct)
	}

	resp, err := h.client.Do(upstreamReq)
	if err != nil {
		server.WriteError(w, http.StatusBadGateway, "Upstream request failed: "+err.Error(), "api_error", "upstream_error")
		return
	}
	defer resp.Body.Close()

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (h *ProxyHandler) RegisterRoutes(mux *http.ServeMux) {
	auth := func(handler http.HandlerFunc) http.Handler {
		return h.DownstreamAuthMiddleware(handler)
	}

	mux.Handle("GET /v1/models", auth(h.HandleModels))
	mux.Handle("POST /v1/chat/completions", auth(h.HandleChat))
	// Fallback catch-all for any other /v1/ endpoint
	mux.Handle("/v1/", auth(h.HandleGenericPassthrough))
}
