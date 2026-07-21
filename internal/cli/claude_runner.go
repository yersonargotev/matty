package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"

	"github.com/yersonargotev/packy/internal/claudecode"
)

// execClaudeRunner is the bounded output-capturing boundary required by
// Claude's version and official user-MCP effects. Command.String remains the
// only renderable description, so environment values never enter errors.
type execClaudeRunner struct{}

func (execClaudeRunner) Run(ctx context.Context, command claudecode.Command) claudecode.Result {
	if command.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, command.Timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, command.Executable, command.Args...)
	cmd.Env = append(os.Environ(), command.Env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	result := claudecode.Result{Stdout: stdout.String(), Stderr: stderr.String(), Err: err, TimedOut: ctx.Err() == context.DeadlineExceeded}
	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
	}
	return result
}

var _ claudecode.Runner = execClaudeRunner{}
