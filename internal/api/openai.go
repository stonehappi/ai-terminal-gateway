// OpenAI-compatible endpoints (/v1/chat/completions, /v1/models) so external
// harnesses — Hermes Agent, OpenClaw, or any OpenAI-SDK client — can use this
// gateway as a model provider by pointing their custom-provider base URL at
// http://<host>:<port>/v1 with a gateway API key.
//
// These endpoints are a plain LLM passthrough: the flattened conversation goes
// straight to the selected generation CLI and its reply is returned verbatim.
// No answer/script decision is made and nothing runs in the sandbox — that
// behavior stays on POST /v1/run. Structured tool/function calling is not
// supported (the backing CLIs return plain text), so harness features that
// depend on OpenAI tool_calls will not activate through this provider.
package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/stonehappi/ai-terminal-gateway/internal/llm"
)

// chatMessage is one OpenAI-style conversation message. Content is usually a
// string, but the spec also allows an array of typed parts — both are accepted.
type chatMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// text returns the message content as plain text, joining any text parts.
func (m chatMessage) text() string {
	var s string
	if err := json.Unmarshal(m.Content, &s); err == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(m.Content, &parts); err == nil {
		var b strings.Builder
		for _, p := range parts {
			if p.Type == "" || p.Type == "text" {
				b.WriteString(p.Text)
			}
		}
		return b.String()
	}
	return ""
}

type chatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type chatCompletionChoice struct {
	Index        int             `json:"index"`
	Message      *assistantReply `json:"message,omitempty"` // non-streaming
	Delta        *assistantReply `json:"delta,omitempty"`   // streaming chunks
	FinishReason *string         `json:"finish_reason"`
}

type assistantReply struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

type chatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []chatCompletionChoice `json:"choices"`
	Usage   *usage                 `json:"usage,omitempty"`
}

// usage is a rough character-based estimate (~4 chars/token): the CLIs don't
// report real token counts, but harnesses render something instead of nulls.
type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func newUsage(prompt, completion string) *usage {
	p, c := len(prompt)/4, len(completion)/4
	return &usage{PromptTokens: p, CompletionTokens: c, TotalTokens: p + c}
}

// handleChatCompletions implements POST /v1/chat/completions.
func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	var req chatCompletionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error")
		return
	}
	if len(req.Messages) == 0 {
		writeOpenAIError(w, http.StatusBadRequest, "messages is required", "invalid_request_error")
		return
	}

	provider, client := s.resolveProvider(req.Model)
	prompt := flattenMessages(req.Messages)

	genCtx, cancel := contextWithGenTimeout(r)
	defer cancel()

	reply, err := client.Complete(genCtx, prompt)
	if err != nil {
		s.log.Error("chat completion failed", "provider", provider, "err", err)
		writeOpenAIError(w, http.StatusBadGateway, "upstream generation failed: "+err.Error(), "api_error")
		return
	}
	reply = strings.TrimSpace(reply)

	// Echo the caller's model name back if it sent one; harnesses key responses
	// (and cost tables) off the id they configured.
	model := req.Model
	if model == "" {
		model = provider
	}
	id := "chatcmpl-" + randomHex(12)
	created := time.Now().Unix()
	stop := "stop"

	if req.Stream {
		streamChatCompletion(w, id, created, model, reply)
		return
	}

	writeJSON(w, http.StatusOK, chatCompletionResponse{
		ID:      id,
		Object:  "chat.completion",
		Created: created,
		Model:   model,
		Choices: []chatCompletionChoice{{
			Message:      &assistantReply{Role: "assistant", Content: reply},
			FinishReason: &stop,
		}},
		Usage: newUsage(prompt, reply),
	})
}

// streamChatCompletion answers a stream:true request with SSE. The backing CLI
// only yields the full reply at once, so this emits one role chunk, one content
// chunk, a finish chunk, and [DONE] — a valid stream any OpenAI client accepts.
func streamChatCompletion(w http.ResponseWriter, id string, created int64, model, reply string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, _ := w.(http.Flusher)
	send := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if flusher != nil {
			flusher.Flush()
		}
	}

	chunk := func(delta *assistantReply, finish *string) chatCompletionResponse {
		return chatCompletionResponse{
			ID: id, Object: "chat.completion.chunk", Created: created, Model: model,
			Choices: []chatCompletionChoice{{Delta: delta, FinishReason: finish}},
		}
	}

	stop := "stop"
	send(chunk(&assistantReply{Role: "assistant"}, nil))
	send(chunk(&assistantReply{Content: reply}, nil))
	send(chunk(&assistantReply{}, &stop))
	fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

// handleModels implements GET /v1/models, which harnesses (e.g. Hermes model
// discovery) query to list what they can select. Each registered generation
// provider is exposed as a model id, default provider first.
func (s *Server) handleModels(w http.ResponseWriter, _ *http.Request) {
	names := make([]string, 0, len(s.llms))
	for name := range s.llms {
		if name != s.defaultProvider {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	names = append([]string{s.defaultProvider}, names...)

	type model struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		OwnedBy string `json:"owned_by"`
	}
	created := time.Now().Unix()
	data := make([]model, 0, len(names))
	for _, name := range names {
		data = append(data, model{ID: name, Object: "model", Created: created, OwnedBy: "ai-gateway"})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

// resolveProvider maps an OpenAI "model" field to a generation client. It
// accepts a bare provider name ("claude"), a harness-prefixed id
// ("ai-gateway/claude"), and falls back to the default provider for anything
// else — harnesses often send opaque model ids they were configured with.
func (s *Server) resolveProvider(model string) (string, *llm.Client) {
	name := strings.ToLower(strings.TrimSpace(model))
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if c, ok := s.llms[name]; ok {
		return name, c
	}
	return s.defaultProvider, s.llms[s.defaultProvider]
}

// flattenMessages converts an OpenAI messages array into the single prompt the
// CLIs accept. System text leads; a lone user message passes through bare, and
// multi-turn history becomes a labeled transcript ending at the model's turn.
func flattenMessages(msgs []chatMessage) string {
	var system, turns []string
	lastUserOnly := true
	for _, m := range msgs {
		text := strings.TrimSpace(m.text())
		if text == "" {
			continue
		}
		switch strings.ToLower(m.Role) {
		case "system", "developer":
			system = append(system, text)
		case "assistant":
			lastUserOnly = false
			turns = append(turns, "Assistant: "+text)
		default: // "user", "tool", anything else
			turns = append(turns, "User: "+text)
		}
	}

	var b strings.Builder
	if len(system) > 0 {
		b.WriteString(strings.Join(system, "\n\n"))
		b.WriteString("\n\n")
	}
	if lastUserOnly && len(turns) == 1 {
		b.WriteString(strings.TrimPrefix(turns[0], "User: "))
		return b.String()
	}
	b.WriteString(strings.Join(turns, "\n\n"))
	b.WriteString("\n\nAssistant:")
	return b.String()
}

// writeOpenAIError writes an error in the shape OpenAI SDK clients parse.
func writeOpenAIError(w http.ResponseWriter, status int, msg, typ string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"message": msg, "type": typ},
	})
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
