package cli

import (
	"context"
	"os"
	"os/exec"
)

// Runner is the seam for external tools such as brew, engram, codex, and
// opencode. Commands receive it through Options so tests can inject a fake.
type Runner interface {
	LookPath(name string) (string, error)
	Run(ctx context.Context, name string, args ...string) error
}

type execRunner struct{}

func (execRunner) LookPath(name string) (string, error) { return exec.LookPath(name) }

func (execRunner) Run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
