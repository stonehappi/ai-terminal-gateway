// Package sandbox executes generated scripts in an isolated "cloud terminal".
//
// The Docker backend runs each script in an ephemeral container with no network
// access and constrained memory/CPU/pids — this is the intended production
// mode. The local backend runs the interpreter directly on the host and exists
// only for development on machines without Docker; it provides NO isolation.
package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Request is a script to execute.
type Request struct {
	Language string // "python" or "bash"
	Script   string
}

// Result captures the outcome of an execution.
type Result struct {
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exit_code"`
	TimedOut   bool   `json:"timed_out"`
	DurationMs int64  `json:"duration_ms"`
}

// Executor runs a script and returns its output.
type Executor interface {
	Execute(ctx context.Context, req Request) (*Result, error)
}

type langSpec struct {
	filename string
	image    string
	// argv, relative to the script path placeholder "%s".
	interp []string
}

func specFor(language, pythonImage, bashImage string) (langSpec, error) {
	switch language {
	case "python":
		return langSpec{filename: "script.py", image: pythonImage, interp: []string{"python"}}, nil
	case "bash":
		return langSpec{filename: "script.sh", image: bashImage, interp: []string{"bash"}}, nil
	default:
		return langSpec{}, fmt.Errorf("unsupported language %q", language)
	}
}

// writeScript writes the script to a fresh temp dir and returns the dir + spec.
func writeScript(req Request, pythonImage, bashImage string) (dir string, spec langSpec, err error) {
	spec, err = specFor(req.Language, pythonImage, bashImage)
	if err != nil {
		return "", spec, err
	}
	dir, err = os.MkdirTemp("", "aigw-sandbox-")
	if err != nil {
		return "", spec, fmt.Errorf("create temp dir: %w", err)
	}
	if err = os.WriteFile(filepath.Join(dir, spec.filename), []byte(req.Script), 0o600); err != nil {
		os.RemoveAll(dir)
		return "", spec, fmt.Errorf("write script: %w", err)
	}
	return dir, spec, nil
}

// run executes cmd, capturing stdout/stderr, and classifies timeouts.
func run(ctx context.Context, timeout time.Duration, name string, args ...string) *Result {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	res := &Result{
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		DurationMs: time.Since(start).Milliseconds(),
	}

	if ctx.Err() == context.DeadlineExceeded {
		res.TimedOut = true
		res.ExitCode = -1
		return res
	}

	var exitErr *exec.ExitError
	switch {
	case err == nil:
		res.ExitCode = 0
	case errors.As(err, &exitErr):
		res.ExitCode = exitErr.ExitCode()
	default:
		// Could not start the process at all (e.g. docker/interpreter missing).
		res.ExitCode = -1
		if res.Stderr == "" {
			res.Stderr = err.Error()
		} else {
			res.Stderr += "\n" + err.Error()
		}
	}
	return res
}
