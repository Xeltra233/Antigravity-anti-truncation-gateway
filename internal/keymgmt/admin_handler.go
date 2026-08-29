package keymgmt

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"antigravity-gateway/internal/server"
)

type AdminHandler struct {
	adminKey string
	mgr      *Manager
}

func NewAdminHandler(adminKey string, mgr *Manager) *AdminHandler {
	return &AdminHandler{
		adminKey: adminKey,
		mgr:      mgr,
	}
}

func (h *AdminHandler) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			server.WriteError(w, http.StatusUnauthorized, "Missing or invalid Authorization header", "auth_error", "unauthorized")
			return
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(h.adminKey)) != 1 {
			server.WriteError(w, http.StatusUnauthorized, "Invalid admin API key", "auth_error", "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

type CreateKeyRequest struct {
	Name          string   `json:"name"`
	ExpiresAt     int64    `json:"expires_at"`
	AllowedModels []string `json:"allowed_models"`
}

func (h *AdminHandler) HandleCreateKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.WriteError(w, http.StatusMethodNotAllowed, "Method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}

	var req CreateKeyRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			server.WriteError(w, http.StatusBadRequest, "Invalid JSON body", "invalid_request_error", "bad_request")
			return
		}
	}

	res, err := h.mgr.CreateKey(req.Name, req.ExpiresAt, req.AllowedModels)
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "Failed to create key", "api_error", "internal_error")
		return
	}

	server.WriteJSON(w, http.StatusCreated, res)
}

func (h *AdminHandler) HandleListKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		server.WriteError(w, http.StatusMethodNotAllowed, "Method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}

	keys := h.mgr.ListKeys()
	server.WriteJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   keys,
	})
}

func (h *AdminHandler) HandleRevokeKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.WriteError(w, http.StatusMethodNotAllowed, "Method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}

	// Path: /admin/keys/{id}/revoke
	path := strings.TrimPrefix(r.URL.Path, "/admin/keys/")
	keyID := strings.TrimSuffix(path, "/revoke")
	keyID = strings.Trim(keyID, "/")

	if keyID == "" {
		server.WriteError(w, http.StatusBadRequest, "Key ID required in URL path", "invalid_request_error", "missing_key_id")
		return
	}

	err := h.mgr.RevokeKey(keyID)
	if err != nil {
		if errors.Is(err, ErrKeyNotFound) {
			server.WriteError(w, http.StatusNotFound, "Key not found", "invalid_request_error", "key_not_found")
			return
		}
		if errors.Is(err, ErrStaticKey) {
			server.WriteError(w, http.StatusForbidden, "Cannot revoke static key", "invalid_request_error", "static_key_immutable")
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "Failed to revoke key", "api_error", "internal_error")
		return
	}

	server.WriteJSON(w, http.StatusOK, map[string]any{
		"id":     keyID,
		"status": "revoked",
	})
}

func (h *AdminHandler) RegisterRoutes(mux *http.ServeMux) {
	adminAuth := func(handler http.HandlerFunc) http.Handler {
		return h.AuthMiddleware(handler)
	}

	mux.Handle("POST /admin/keys", adminAuth(h.HandleCreateKey))
	mux.Handle("GET /admin/keys", adminAuth(h.HandleListKeys))
	mux.Handle("POST /admin/keys/{id}/revoke", adminAuth(h.HandleRevokeKey))
}
