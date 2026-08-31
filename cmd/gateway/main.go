package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"antigravity-gateway/pkg/gateway"
)

func main() {
	if len(os.Args) > 1 {
		arg := strings.ToLower(os.Args[1])
		if arg == "-v" || arg == "-version" || arg == "--version" || arg == "version" {
			fmt.Printf("Antigravity Gateway version %s (commit: %s, built: %s)\n", gateway.Version, gateway.GitCommit, gateway.BuildTime)
			os.Exit(0)
		}
	}

	engine, err := gateway.StartEngine(os.Getenv)
	if err != nil {
		slog.Error("server fatal error", "err", err)
		os.Exit(1)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	slog.Info("received termination signal", "signal", sig.String())

	if err := engine.Stop(); err != nil {
		slog.Error("shutdown error", "err", err)
		os.Exit(1)
	}
}
