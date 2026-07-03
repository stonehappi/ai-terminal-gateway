package sandbox

import (
	"context"
	"os"
	"time"
)

// DockerExecutor runs scripts inside ephemeral, network-isolated containers.
type DockerExecutor struct {
	Timeout     time.Duration
	PythonImage string
	BashImage   string
	MemoryLimit string // e.g. "256m"
	CPULimit    string // e.g. "1"
}

// Execute writes the script to a temp dir, mounts it into a locked-down
// container, and runs it. The container is removed automatically (--rm).
func (e *DockerExecutor) Execute(ctx context.Context, req Request) (*Result, error) {
	dir, spec, err := writeScript(req, e.PythonImage, e.BashImage)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	args := []string{
		"run", "--rm",
		"--network", "none", // no outbound network
		"--memory", e.MemoryLimit,
		"--cpus", e.CPULimit,
		"--pids-limit", "128",
		"--read-only",                     // read-only root filesystem
		"--tmpfs", "/tmp:rw,size=64m",      // writable scratch space
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"-v", dir + ":/work",
		"-w", "/work",
		spec.image,
	}
	args = append(args, spec.interp...)
	args = append(args, "/work/"+spec.filename)

	return run(ctx, e.Timeout, "docker", args...), nil
}
