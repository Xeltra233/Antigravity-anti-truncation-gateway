package gateway

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
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
	Version   = "1.0.0"
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

type Engine struct {
	cfg     *config.Config
	srv     *server.Server
	keyMgr  *keymgmt.Manager
	cancel  context.CancelFunc
	ctx     context.Context
	mu      sync.Mutex
	running bool
}

func StartEngine(getenv func(string) string) (*Engine, error) {
	cfg, err := config.Load(getenv)
	if err != nil {
		return nil, fmt.Errorf("configuration error: %w", err)
	}

	logger.Init(cfg.LogLevel, os.Stdout)
	slog.Info("starting antigravity-gateway engine",
		"version", Version,
		"git_commit", GitCommit,
		"build_time", BuildTime,
		"port", cfg.Port,
		"upstream", cfg.UpstreamBaseURL,
		"wrapper_mode", cfg.WrapperMode,
		"recovery_policy", cfg.RecoveryPolicy,
		"empty_retries", cfg.UpstreamEmptyRetries,
	)

	keyMgr, err := keymgmt.NewManager(cfg.KeyDBPath, cfg.KeyHMACSecret, cfg.StaticKeys)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize key manager: %w", err)
	}

	upstreamClient := proxy.NewUpstreamClient(cfg)
	orch := recovery.NewOrchestrator(cfg, upstreamClient, keyMgr)
	proxyHandler := proxy.NewProxyHandler(cfg, keyMgr, upstreamClient)
	proxyHandler.SetChatHandler(orch.HandleChatCompletions)
	adminHandler := keymgmt.NewAdminHandler(cfg.AdminAPIKey, keyMgr)

	mux := http.NewServeMux()
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
	proxyHandler.RegisterRoutes(mux)
	adminHandler.RegisterRoutes(mux)

	handlerWithMiddlewares := detailedLoggingMiddleware(mux)
	srv := server.New(cfg, handlerWithMiddlewares)
	if err := srv.Start(); err != nil {
		keyMgr.Close()
		return nil, fmt.Errorf("failed to start server listener on port %d: %w", cfg.Port, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	engine := &Engine{
		cfg:     cfg,
		srv:     srv,
		keyMgr:  keyMgr,
		cancel:  cancel,
		ctx:     ctx,
		running: true,
	}

	return engine, nil
}

func (e *Engine) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.running {
		return nil
	}
	e.running = false
	e.cancel()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var err error
	if e.srv != nil {
		err = e.srv.Shutdown(shutdownCtx)
	}
	if e.keyMgr != nil {
		_ = e.keyMgr.Close()
	}
	slog.Info("gateway engine stopped cleanly")
	return err
}

func (e *Engine) IsRunning() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

func (e *Engine) Addr() string {
	if e.srv != nil {
		return e.srv.Addr()
	}
	return ""
}
