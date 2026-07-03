// Package api exposes the gateway's HTTP surface.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/stone-siit/ai-terminal-gateway/internal/config"
	"github.com/stone-siit/ai-terminal-gateway/internal/llm"
	"github.com/stone-siit/ai-terminal-gateway/internal/sandbox"
)

// Server ties together the LLM client and sandbox executor behind HTTP handlers.
type Server struct {
	cfg      *config.Config
	llm      *llm.Client
	executor sandbox.Executor
	log      *slog.Logger
}

// NewServer constructs a Server.
func NewServer(cfg *config.Config, l *llm.Client, e sandbox.Executor, log *slog.Logger) *Server {
	return &Server{cfg: cfg, llm: l, executor: e, log: log}
}

// Handler returns the root HTTP handler with all routes wired up.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.Handle("POST /v1/run", s.authMiddleware(http.HandlerFunc(s.handleRun)))
	return logMiddleware(s.log, mux)
}

type runRequest struct {
	Prompt   string `json:"prompt"`
	Language string `json:"language,omitempty"` // optional: "python" or "bash"
}

type runResponse struct {
	Mode        string          `json:"mode"`
	Answer      string          `json:"answer,omitempty"`
	Language    string          `json:"language,omitempty"`
	Script      string          `json:"script,omitempty"`
	Explanation string          `json:"explanation,omitempty"`
	Execution   *sandbox.Result `json:"execution,omitempty"`
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	var req runRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}
	if req.Language != "" && req.Language != "python" && req.Language != "bash" {
		writeError(w, http.StatusBadRequest, "language must be 'python' or 'bash'")
		return
	}

	// Generate the script from the prompt.
	genCtx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	gen, err := s.llm.Generate(genCtx, req.Prompt, req.Language)
	if err != nil {
		s.log.Error("generation failed", "err", err)
		writeError(w, http.StatusBadGateway, "failed to handle request: "+err.Error())
		return
	}

	// Assistant mode: answer directly, no sandbox execution.
	if gen.Mode == llm.ModeAnswer {
		writeJSON(w, http.StatusOK, runResponse{
			Mode:   llm.ModeAnswer,
			Answer: gen.Answer,
		})
		return
	}

	// Script mode: a user-specified language takes precedence over the model's.
	lang := gen.Language
	if req.Language != "" {
		lang = req.Language
	}

	result, err := s.executor.Execute(r.Context(), sandbox.Request{
		Language: lang,
		Script:   gen.Script,
	})
	if err != nil {
		s.log.Error("sandbox execution failed", "err", err)
		writeError(w, http.StatusBadGateway, "failed to execute script: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, runResponse{
		Mode:        llm.ModeScript,
		Language:    lang,
		Script:      gen.Script,
		Explanation: gen.Explanation,
		Execution:   result,
	})
}

// authMiddleware enforces Bearer-token auth against the configured gateway keys.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.AuthDisabled() {
			next.ServeHTTP(w, r)
			return
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if _, ok := s.cfg.GatewayAPIKeys[token]; !ok || token == "" {
			writeError(w, http.StatusUnauthorized, "missing or invalid API key")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func logMiddleware(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
