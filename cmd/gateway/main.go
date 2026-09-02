package main

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"antigravity-gateway/internal/config"
	"antigravity-gateway/internal/keymgmt"
	"antigravity-gateway/internal/logger"
	"antigravity-gateway/internal/metrics"
	"antigravity-gateway/internal/proxy"
	"antigravity-gateway/internal/recovery"
	"antigravity-gateway/internal/server"
)

var (
	Version   = "1.0.6"
	GitCommit = "none"
	BuildTime = "unknown"
)

type responseLogger struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
	bodyBuf      bytes.Buffer
}

func (rl *responseLogger) WriteHeader(code int) {
	rl.statusCode = code
	rl.ResponseWriter.WriteHeader(code)
}

func (rl *responseLogger) Write(b []byte) (int, error) {
	if rl.statusCode == 0 {
		rl.statusCode = http.StatusOK
	}
	rl.bytesWritten += int64(len(b))
	if rl.bodyBuf.Len() < 2048 {
		remaining := 2048 - rl.bodyBuf.Len()
		if len(b) > remaining {
			rl.bodyBuf.Write(b[:remaining])
		} else {
			rl.bodyBuf.Write(b)
		}
	}
	return rl.ResponseWriter.Write(b)
}

func (rl *responseLogger) Flush() {
	if f, ok := rl.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func detailedLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			slog.Info("HTTP OPTIONS preflight handled", "path", r.URL.Path, "remote", r.RemoteAddr)
			return
		}

		// Read & preview request body (up to 4KB)
		var reqBodyPreview string
		if r.Body != nil && r.ContentLength != 0 {
			buf, err := io.ReadAll(io.LimitReader(r.Body, 4096))
			if err == nil && len(buf) > 0 {
				reqBodyPreview = string(buf)
				r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(buf), r.Body))
			}
		}

		authHeader := r.Header.Get("Authorization")
		maskedAuth := ""
		if len(authHeader) > 15 {
			maskedAuth = authHeader[:10] + "..." + authHeader[len(authHeader)-4:]
		} else if authHeader != "" {
			maskedAuth = "***"
		}

		slog.Info(">>> INCOMING HTTP REQUEST",
			"method", r.Method,
			"path", r.URL.Path,
			"query", r.URL.RawQuery,
			"remote", r.RemoteAddr,
			"content_type", r.Header.Get("Content-Type"),
			"auth", maskedAuth,
			"body_preview", strings.ReplaceAll(strings.ReplaceAll(reqBodyPreview, "\n", " "), "\r", " "),
		)

		start := time.Now()
		rl := &responseLogger{
			ResponseWriter: w,
			statusCode:     0,
		}

		next.ServeHTTP(rl, r)

		respPreview := strings.TrimSpace(rl.bodyBuf.String())
		if len(respPreview) > 500 {
			respPreview = respPreview[:500] + "...(truncated)"
		}

		slog.Info("<<< OUTGOING HTTP RESPONSE",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rl.statusCode,
			"bytes", rl.bytesWritten,
			"duration_ms", time.Since(start).Milliseconds(),
			"resp_preview", strings.ReplaceAll(strings.ReplaceAll(respPreview, "\n", " "), "\r", " "),
		)
	})
}

func main() {
	if len(os.Args) > 1 {
		arg := strings.ToLower(os.Args[1])
		if arg == "-v" || arg == "-version" || arg == "--version" || arg == "version" {
			fmt.Printf("Antigravity Gateway version %s (commit: %s, built: %s)\n", Version, GitCommit, BuildTime)
			os.Exit(0)
		}
	}

	cfg, err := config.LoadFromEnv()
	if err != nil {
		slog.Error("configuration error", "err", err)
		os.Exit(1)
	}

	logger.Init(cfg.LogLevel, os.Stdout)
	if cfg.EnvFileLoaded != "" {
		slog.Info("loaded configuration from .env file", "path", cfg.EnvFileLoaded)
	}
	slog.Info("starting antigravity-gateway",
		"version", Version,
		"git_commit", GitCommit,
		"build_time", BuildTime,
		"port", cfg.Port,
		"upstream", cfg.UpstreamBaseURL,
		"env_file", cfg.EnvFileLoaded,
		"wrapper_mode", cfg.WrapperMode,
		"recovery_policy", cfg.RecoveryPolicy,
		"empty_retries", cfg.UpstreamEmptyRetries,
	)

	// 1. Initialize Key Manager (SQLite + in-memory snapshot)
	keyMgr, err := keymgmt.NewManager(cfg.KeyDBPath, cfg.KeyHMACSecret, cfg.StaticKeys)
	if err != nil {
		slog.Error("failed to initialize key manager", "err", err)
		os.Exit(1)
	}
	defer keyMgr.Close()

	// 2. Initialize Upstream Client
	upstreamClient := proxy.NewUpstreamClient(cfg)

	// 3. Initialize Orchestrator
	orch := recovery.NewOrchestrator(cfg, upstreamClient, keyMgr)

	// 4. Initialize Proxy Handler
	proxyHandler := proxy.NewProxyHandler(cfg, keyMgr, upstreamClient)
	proxyHandler.SetChatHandler(orch.HandleChatCompletions)

	// 5. Initialize Admin Handler
	adminHandler := keymgmt.NewAdminHandler(cfg.AdminAPIKey, keyMgr)

	// 6. Register HTTP Routes
	mux := http.NewServeMux()

	// Health & Metrics
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		server.WriteJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"version": Version,
		})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		server.WriteJSON(w, http.StatusOK, map[string]string{
			"status":  "ready",
			"version": Version,
		})
	})
	mux.HandleFunc("GET /metrics", metrics.Default.Handler())

	// Proxy Routes (/v1/models, /v1/chat/completions, /v1/*)
	proxyHandler.RegisterRoutes(mux)

	// Admin Routes (/admin/keys)
	adminHandler.RegisterRoutes(mux)

	handlerWithMiddlewares := detailedLoggingMiddleware(mux)

	// 7. Run Server with Graceful Shutdown
	if err := server.RunWithSignals(cfg, handlerWithMiddlewares); err != nil {
		slog.Error("server fatal error", "err", err)
		os.Exit(1)
	}
}
