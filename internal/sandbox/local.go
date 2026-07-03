package sandbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// LocalExecutor runs the interpreter directly on the host. It provides NO
// isolation and is intended only for local development without Docker.
type LocalExecutor struct {
	Timeout time.Duration
}

// Execute writes the script to a temp dir and runs it with the host interpreter.
func (e *LocalExecutor) Execute(ctx context.Context, req Request) (*Result, error) {
	// Reuse the same spec/filename logic; images are unused locally.
	dir, spec, err := writeScript(req, "", "")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	interp, err := resolveInterpreter(spec.interp[0])
	if err != nil {
		return &Result{ExitCode: -1, Stderr: err.Error()}, nil
	}

	return run(ctx, e.Timeout, interp, filepath.Join(dir, spec.filename)), nil
}

// resolveInterpreter finds a usable interpreter binary, accounting for the fact
// that "python" is often "python3" and bash may be absent on Windows.
func resolveInterpreter(name string) (string, error) {
	candidates := []string{name}
	if name == "python" {
		candidates = []string{"python3", "python"}
	}
	var lastErr error
	for _, c := range candidates {
		if p, err := exec.LookPath(c); err == nil {
			return p, nil
		} else {
			lastErr = err
		}
	}
	return "", lastErr
}
