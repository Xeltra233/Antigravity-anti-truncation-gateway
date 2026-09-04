package main

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDetailedLoggingMiddlewarePreflight(t *testing.T) {
	handler := detailedLoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("preflight request must not reach the application handler")
	}))

	req := httptest.NewRequest(http.MethodOptions, "/v1/chat/completions", nil)
	req.Header.Set("Origin", "https://client.example")
	req.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("allow origin = %q, want wildcard for token-auth API", got)
	}
	allowedHeaders := strings.ToLower(recorder.Header().Get("Access-Control-Allow-Headers"))
	for _, required := range []string{"authorization", "content-type", "x-request-id"} {
		if !strings.Contains(allowedHeaders, required) {
			t.Errorf("allow headers %q does not contain %q", allowedHeaders, required)
		}
	}
	if got := recorder.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("credentialed wildcard CORS is invalid; got Access-Control-Allow-Credentials=%q", got)
	}
}

func TestDetailedLoggingMiddlewareDoesNotLogBodies(t *testing.T) {
	const requestSecret = "request-body-secret-value"
	const responseSecret = "response-body-secret-value"
	const bearerSecret = "downstream-bearer-secret-value"

	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	handler := detailedLoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if string(body) != requestSecret {
			t.Fatalf("handler received body %q, want %q", body, requestSecret)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(responseSecret))
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?debug=false", strings.NewReader(requestSecret))
	req.Header.Set("Authorization", "Bearer "+bearerSecret)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("response status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	if recorder.Body.String() != responseSecret {
		t.Fatalf("response body = %q, want %q", recorder.Body.String(), responseSecret)
	}
	logOutput := logs.String()
	for _, secret := range []string{requestSecret, responseSecret, bearerSecret} {
		if strings.Contains(logOutput, secret) {
			t.Errorf("sensitive value %q leaked to request logs: %s", secret, logOutput)
		}
	}
	if !strings.Contains(logOutput, `"status":201`) {
		t.Errorf("response status missing from logs: %s", logOutput)
	}
}

func TestResponseLoggerIgnoresDuplicateWriteHeader(t *testing.T) {
	recorder := httptest.NewRecorder()
	logger := &responseLogger{ResponseWriter: recorder}

	logger.WriteHeader(http.StatusCreated)
	logger.WriteHeader(http.StatusInternalServerError)
	_, _ = logger.Write([]byte("ok"))

	if logger.statusCode != http.StatusCreated {
		t.Fatalf("logged status = %d, want first status %d", logger.statusCode, http.StatusCreated)
	}
	if recorder.Code != http.StatusCreated {
		t.Fatalf("wire status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	if logger.bytesWritten != 2 {
		t.Fatalf("logged bytes = %d, want 2", logger.bytesWritten)
	}
}
