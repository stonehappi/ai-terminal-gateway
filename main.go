// Command ai-gateway-api runs an HTTP gateway that turns a natural-language
// prompt into a script (via Claude), executes it in an isolated cloud terminal
// sandbox, and returns the output.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/stone-siit/ai-terminal-gateway/internal/api"
	"github.com/stone-siit/ai-terminal-gateway/internal/config"
	"github.com/stone-siit/ai-terminal-gateway/internal/llm"
	"github.com/stone-siit/ai-terminal-gateway/internal/sandbox"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		log.Error("configuration error", "err", err)
		os.Exit(1)
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

	// Select the generation CLI backend.
	genBin, genModel := cfg.ClaudeBin, cfg.ClaudeModel
	switch cfg.Provider {
	case llm.ProviderAgy:
		genBin, genModel = cfg.AgyBin, cfg.AgyModel
	case llm.ProviderCodex:
		genBin, genModel = cfg.CodexBin, cfg.CodexModel
	}
	srv := api.NewServer(cfg, llm.New(cfg.Provider, genBin, genModel), executor, log)

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("gateway listening", "port", cfg.Port, "backend", cfg.SandboxBackend, "provider", cfg.Provider, "model", genModel)
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
