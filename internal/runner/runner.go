package runner

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/ols/ols-cli/internal/apperr"
)

// Result captures executed command outputs.
type Result struct {
	Stdout string
	Stderr string
}

// Runner executes system commands safely.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (Result, error)
}

// ExecRunner is the default command runner backed by os/exec.
type ExecRunner struct{}

func NewExecRunner() ExecRunner {
	return ExecRunner{}
}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) (Result, error) {
	cmd := exec.CommandContext(ctx, name, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return Result{Stdout: stdout.String(), Stderr: stderr.String()}, apperr.Wrap(
			apperr.CodeCommand,
			fmt.Sprintf("command failed: %s %s", name, strings.Join(args, " ")),
			err,
		)
	}

	return Result{Stdout: stdout.String(), Stderr: stderr.String()}, nil
}
