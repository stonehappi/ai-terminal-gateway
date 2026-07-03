// Package llm handles a natural-language request by driving an agentic coding
// CLI in headless print mode. The CLI decides whether to answer directly
// (assistant mode) or to produce a script that the gateway runs in a sandbox.
//
// Three CLI providers are supported, selected at construction time:
//
//   - "claude" — the Claude Code CLI. Reads the prompt from stdin, is invoked
//     with `--output-format json`, and wraps the model's final text in a JSON
//     envelope that this package unwraps. Auth comes from your local Claude Code
//     login — no ANTHROPIC_API_KEY needed.
//   - "agy" — the agy CLI. Takes the prompt as a `--print` argument and emits
//     the model's raw text directly (no JSON envelope). Auth comes from your
//     local agy login.
//   - "codex" — the OpenAI Codex CLI. Runs `codex exec <prompt>` headlessly and
//     prints the model's final message as raw text (no JSON envelope). Auth
//     comes from your local codex login.
//
// The request is not limited to code: the model may reply in "answer" mode
// (a plain-text business-assistant response) or "script" mode (code the gateway
// runs in a sandbox). Either way the model returns a single JSON decision
// object, which this package extracts and normalizes.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// Provider identifies which CLI backend generates responses.
const (
	ProviderClaude = "claude"
	ProviderAgy    = "agy"
	ProviderCodex  = "codex"
)

// Mode is how the gateway should handle a request.
const (
	ModeAnswer = "answer" // reply directly with text, no execution
	ModeScript = "script" // generate a script and run it in the sandbox
)

// Generation is the structured decision Claude returns for a request.
type Generation struct {
	Mode        string `json:"mode"`
	Answer      string `json:"answer"`      // used when Mode == ModeAnswer
	Language    string `json:"language"`    // used when Mode == ModeScript
	Script      string `json:"script"`      // used when Mode == ModeScript
	Explanation string `json:"explanation"` // used when Mode == ModeScript
}

// Client drives an agentic coding CLI (Claude Code, agy, or Codex) in headless
// print mode.
type Client struct {
	provider string // ProviderClaude, ProviderAgy, or ProviderCodex
	bin      string // path/name of the CLI binary
	model    string // optional --model override; empty = CLI default
}

// New builds a Client for the given provider. provider defaults to
// ProviderClaude if empty; bin defaults to the provider's conventional binary
// name if empty.
func New(provider, bin, model string) *Client {
	if provider == "" {
		provider = ProviderClaude
	}
	if bin == "" {
		bin = provider // "claude" or "agy" — the binary shares the provider name
	}
	return &Client{provider: provider, bin: bin, model: model}
}

const instructions = `You are the engine of an AI gateway. The user sends a request below.
First decide how to handle it, then respond with ONE JSON object only (no markdown, no prose).

Choose ONE mode:

1. "answer" — the request is a general question, explanation, advice, or
   conversation that does NOT require running code to produce the result.
   Respond with:
   {"mode": "answer", "answer": "<your full reply as plain text>"}

2. "script" — the request needs computation, data processing, or otherwise
   benefits from executing code to produce the result (e.g. "calculate...",
   "generate N of...", "parse...", "simulate..."). Write a single, self-contained
   script and respond with:
   {"mode": "script", "language": "python" | "bash", "script": "<full source>", "explanation": "<one sentence>"}

Rules for scripts you generate:
- They run inside an isolated sandbox container with NO network access.
- Fully self-contained: no external files, no network calls, no interactive input.
- Only the Python standard library is guaranteed to be available for Python.
- Print the final result to stdout. Do not just define functions.
- Do NOT use any tools, do NOT run or execute anything, do NOT create files — only return the JSON.`

// Generate asks the configured CLI how to handle the request. languageHint, if
// non-empty ("python" or "bash"), forces script mode in that language.
func (c *Client) Generate(ctx context.Context, prompt, languageHint string) (*Generation, error) {
	var b strings.Builder
	b.WriteString(instructions)
	if languageHint != "" {
		b.WriteString("\n\nThe user explicitly requested code, so you MUST use mode \"script\" with language ")
		b.WriteString(languageHint)
		b.WriteString(".")
	}
	b.WriteString("\n\nREQUEST:\n")
	b.WriteString(prompt)

	// Ask the provider's CLI and recover the model's final text response.
	text, err := c.run(ctx, b.String())
	if err != nil {
		return nil, err
	}

	g := extractJSON(text)
	if g == nil {
		return nil, fmt.Errorf("%s did not return a valid decision", c.provider)
	}
	return normalize(g)
}

// run invokes the provider's CLI in print mode and returns the model's final
// text (from which a JSON decision is later extracted).
func (c *Client) run(ctx context.Context, prompt string) (string, error) {
	switch c.provider {
	case ProviderAgy:
		return c.runAgy(ctx, prompt)
	case ProviderCodex:
		return c.runCodex(ctx, prompt)
	default:
		return c.runClaude(ctx, prompt)
	}
}

// runClaude drives the Claude Code CLI, which reads the prompt from stdin and,
// with --output-format json, wraps the final text in a JSON envelope.
func (c *Client) runClaude(ctx context.Context, prompt string) (string, error) {
	args := []string{"-p", "--output-format", "json"}
	if c.model != "" {
		args = append(args, "--model", c.model)
	}

	cmd := exec.CommandContext(ctx, c.bin, args...)
	cmd.Stdin = strings.NewReader(prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("claude cli failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	var env struct {
		IsError bool   `json:"is_error"`
		Result  string `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		return "", fmt.Errorf("could not parse claude output: %w", err)
	}
	if env.IsError {
		return "", fmt.Errorf("claude reported an error: %s", env.Result)
	}
	return env.Result, nil
}

// runAgy drives the agy CLI, which takes the prompt as a --print argument and
// prints the model's raw text (no JSON envelope) to stdout.
func (c *Client) runAgy(ctx context.Context, prompt string) (string, error) {
	args := []string{"--print", prompt}
	if c.model != "" {
		args = append(args, "--model", c.model)
	}

	cmd := exec.CommandContext(ctx, c.bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("agy cli failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// runCodex drives the OpenAI Codex CLI in headless "exec" mode. It prints the
// model's final message as raw text (no JSON envelope) to stdout; banner/log
// noise goes to stderr.
//
// The prompt is fed on stdin with "-" as the prompt argument rather than passed
// as a CLI arg: the prompt contains quotes and braces, and on Windows `codex`
// is an npm .cmd shim whose batch argument parsing mangles those characters.
// stdin avoids all command-line quoting. --skip-git-repo-check lets it run
// outside a git repo (the gateway's working directory is arbitrary).
func (c *Client) runCodex(ctx context.Context, prompt string) (string, error) {
	args := []string{"exec", "--skip-git-repo-check"}
	if c.model != "" {
		args = append(args, "--model", c.model)
	}
	args = append(args, "-") // read the prompt from stdin

	cmd := exec.CommandContext(ctx, c.bin, args...)
	cmd.Stdin = strings.NewReader(prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("codex cli failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func normalize(g *Generation) (*Generation, error) {
	g.Mode = strings.ToLower(strings.TrimSpace(g.Mode))

	// Infer the mode if the model omitted it.
	if g.Mode == "" {
		if strings.TrimSpace(g.Script) != "" {
			g.Mode = ModeScript
		} else {
			g.Mode = ModeAnswer
		}
	}

	switch g.Mode {
	case ModeAnswer:
		if strings.TrimSpace(g.Answer) == "" {
			return nil, fmt.Errorf("answer mode returned an empty answer")
		}
	case ModeScript:
		g.Language = strings.ToLower(strings.TrimSpace(g.Language))
		if g.Language == "" {
			g.Language = "python"
		}
		if g.Language != "python" && g.Language != "bash" {
			return nil, fmt.Errorf("unsupported language %q", g.Language)
		}
		if strings.TrimSpace(g.Script) == "" {
			return nil, fmt.Errorf("script mode returned an empty script")
		}
	default:
		return nil, fmt.Errorf("unknown mode %q", g.Mode)
	}
	return g, nil
}

var jsonFence = regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*\\})\\s*```")

// extractJSON recovers a Generation JSON object from the model's text, whether
// it's bare, fenced in ```json, or surrounded by prose.
func extractJSON(text string) *Generation {
	text = strings.TrimSpace(text)
	candidate := ""
	if m := jsonFence.FindStringSubmatch(text); m != nil {
		candidate = m[1]
	} else if i := strings.Index(text, "{"); i >= 0 {
		if j := strings.LastIndex(text, "}"); j > i {
			candidate = text[i : j+1]
		}
	}
	if candidate == "" {
		return nil
	}
	var g Generation
	if err := json.Unmarshal([]byte(candidate), &g); err != nil {
		return nil
	}
	return &g
}
