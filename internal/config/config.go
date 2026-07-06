// Package config loads gateway configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration for the gateway.
type Config struct {
	// HTTP
	Port string

	// LogFile, if set, is a path the gateway writes its JSON logs to instead of
	// stdout. Required when the binary is built as a Windows GUI app (no console),
	// so logs aren't lost. Empty = log to stdout (default, dev-friendly).
	LogFile string

	// Auth: callers must present one of these as a Bearer token. If empty,
	// auth is disabled (development only) and a warning is logged.
	GatewayAPIKeys map[string]struct{}

	// Generation CLI (drives an agentic coding CLI instead of the API).
	// Provider selects the backend: "claude" (default), "agy", or "codex".
	Provider    string
	ClaudeBin   string // binary name/path, default "claude"
	ClaudeModel string // optional --model override; empty = CLI default
	AgyBin      string // binary name/path, default "agy"
	AgyModel    string // optional --model override; empty = CLI default
	CodexBin    string // binary name/path, default "codex"
	CodexModel  string // optional --model override; empty = CLI default

	// Sandbox
	SandboxBackend string        // "docker" (default) or "local"
	SandboxTimeout time.Duration // wall-clock limit for a single script
	PythonImage    string        // docker image used for python scripts
	BashImage      string        // docker image used for bash scripts
	MemoryLimit    string        // docker --memory, e.g. "256m"
	CPULimit       string        // docker --cpus, e.g. "1"
}

// Load reads configuration from the environment, applying sensible defaults.
func Load() (*Config, error) {
	cfg := &Config{
		Port:           getenv("PORT", "8081"),
		LogFile:        os.Getenv("GATEWAY_LOG_FILE"),
		Provider:       getenv("LLM_PROVIDER", "claude"),
		ClaudeBin:      getenv("CLAUDE_BIN", "claude"),
		ClaudeModel:    os.Getenv("CLAUDE_MODEL"),
		AgyBin:         getenv("AGY_BIN", "agy"),
		AgyModel:       os.Getenv("AGY_MODEL"),
		CodexBin:       getenv("CODEX_BIN", "codex"),
		CodexModel:     os.Getenv("CODEX_MODEL"),
		SandboxBackend: getenv("SANDBOX_BACKEND", "docker"),
		PythonImage:    getenv("SANDBOX_PYTHON_IMAGE", "python:3.12-slim"),
		BashImage:      getenv("SANDBOX_BASH_IMAGE", "bash:5"),
		MemoryLimit:    getenv("SANDBOX_MEMORY", "256m"),
		CPULimit:       getenv("SANDBOX_CPUS", "1"),
		GatewayAPIKeys: map[string]struct{}{},
	}

	timeoutSecs, err := strconv.Atoi(getenv("SANDBOX_TIMEOUT_SECONDS", "30"))
	if err != nil {
		return nil, fmt.Errorf("invalid SANDBOX_TIMEOUT_SECONDS: %w", err)
	}
	cfg.SandboxTimeout = time.Duration(timeoutSecs) * time.Second

	for _, k := range strings.Split(os.Getenv("GATEWAY_API_KEYS"), ",") {
		if k = strings.TrimSpace(k); k != "" {
			cfg.GatewayAPIKeys[k] = struct{}{}
		}
	}

	switch cfg.SandboxBackend {
	case "docker", "local":
	default:
		return nil, fmt.Errorf("invalid SANDBOX_BACKEND %q (want docker|local)", cfg.SandboxBackend)
	}

	switch cfg.Provider {
	case "claude", "agy", "codex":
	default:
		return nil, fmt.Errorf("invalid LLM_PROVIDER %q (want claude|agy|codex)", cfg.Provider)
	}

	return cfg, nil
}

// AuthDisabled reports whether no gateway API keys are configured.
func (c *Config) AuthDisabled() bool {
	return len(c.GatewayAPIKeys) == 0
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
