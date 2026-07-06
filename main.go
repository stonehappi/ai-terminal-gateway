// Command ai-gateway-api runs an HTTP gateway that turns a natural-language
// prompt into a script (via Claude), executes it in an isolated cloud terminal
// sandbox, and returns the output.
package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/stonehappi/ai-terminal-gateway/internal/api"
	"github.com/stonehappi/ai-terminal-gateway/internal/config"
	"github.com/stonehappi/ai-terminal-gateway/internal/llm"
	"github.com/stonehappi/ai-terminal-gateway/internal/sandbox"
)

func main() {
	// Bootstrap logger to stdout so config errors are visible during dev.
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		log.Error("configuration error", "err", err)
		os.Exit(1)
	}

	// If a log file is configured, switch to it. When the binary is built as a
	// Windows GUI app (-H windowsgui) there is no console, so stdout goes
	// nowhere — writing to a file keeps the logs. Fall back to stdout on error.
	var logOut io.Writer = os.Stdout
	var logFileErr error
	if cfg.LogFile != "" {
		if f, ferr := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); ferr != nil {
			logFileErr = ferr
		} else {
			defer f.Close()
			logOut = f
		}
	}
	log = slog.New(slog.NewJSONHandler(logOut, nil))
	if logFileErr != nil {
		log.Warn("could not open GATEWAY_LOG_FILE; logging to stdout", "path", cfg.LogFile, "err", logFileErr)
	}
	if cfg.AuthDisabled() {
		log.Warn("no GATEWAY_API_KEYS configured — /v1/run is UNAUTHENTICATED (dev mode)")
	}

	var executor sandbox.Executor
	switch cfg.SandboxBackend {
	case "local":
		log.Warn("using LOCAL sandbox backend — scripts run on the host with NO isolation")
		executor = &sandbox.LocalExecutor{Timeout: cfg.SandboxTimeout}
	default:
		executor = &sandbox.DockerExecutor{
			Timeout:     cfg.SandboxTimeout,
			PythonImage: cfg.PythonImage,
			BashImage:   cfg.BashImage,
			MemoryLimit: cfg.MemoryLimit,
			CPULimit:    cfg.CPULimit,
		}
	}

	// Build a client for every generation backend. Callers may pick one
	// per-request via the "provider" field; cfg.Provider is the default.
	clients := map[string]*llm.Client{
		llm.ProviderClaude: llm.New(llm.ProviderClaude, cfg.ClaudeBin, cfg.ClaudeModel),
		llm.ProviderAgy:    llm.New(llm.ProviderAgy, cfg.AgyBin, cfg.AgyModel),
		llm.ProviderCodex:  llm.New(llm.ProviderCodex, cfg.CodexBin, cfg.CodexModel),
	}
	srv := api.NewServer(cfg, clients, cfg.Provider, executor, log)

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("gateway listening", "port", cfg.Port, "backend", cfg.SandboxBackend, "default_provider", cfg.Provider)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown on SIGINT/SIGTERM.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Info("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Error("shutdown error", "err", err)
	}
}
