package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

type RedactingHandler struct {
	inner slog.Handler
}

func NewRedactingHandler(inner slog.Handler) *RedactingHandler {
	return &RedactingHandler{inner: inner}
}

func (h *RedactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *RedactingHandler) Handle(ctx context.Context, r slog.Record) error {
	var newAttrs []slog.Attr
	r.Attrs(func(a slog.Attr) bool {
		newAttrs = append(newAttrs, sanitizeAttr(a))
		return true
	})

	newRecord := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	newRecord.AddAttrs(newAttrs...)
	return h.inner.Handle(ctx, newRecord)
}

func (h *RedactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	sanitized := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		sanitized[i] = sanitizeAttr(a)
	}
	return &RedactingHandler{inner: h.inner.WithAttrs(sanitized)}
}

func (h *RedactingHandler) WithGroup(name string) slog.Handler {
	return &RedactingHandler{inner: h.inner.WithGroup(name)}
}

func sanitizeAttr(a slog.Attr) slog.Attr {
	key := strings.ToLower(a.Key)
	if strings.Contains(key, "key") || strings.Contains(key, "secret") || strings.Contains(key, "token") ||
		strings.Contains(key, "auth") || strings.Contains(key, "prompt") || strings.Contains(key, "password") {
		if a.Key == "key_id" || a.Key == "key_prefix" || a.Key == "auth_mode" {
			return a
		}
		return slog.String(a.Key, "[REDACTED]")
	}
	return a
}

func Init(levelStr string, w io.Writer) *slog.Logger {
	if w == nil {
		w = os.Stdout
	}
	var level slog.Level
	switch strings.ToLower(levelStr) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}
	jsonHandler := slog.NewJSONHandler(w, opts)
	handler := NewRedactingHandler(jsonHandler)
	l := slog.New(handler)
	slog.SetDefault(l)
	return l
}
