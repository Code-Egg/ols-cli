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
		safeArgs := redactCommandArgs(args)
		return Result{Stdout: stdout.String(), Stderr: stderr.String()}, apperr.Wrap(
			apperr.CodeCommand,
			fmt.Sprintf("command failed: %s %s", name, strings.Join(safeArgs, " ")),
			err,
		)
	}

	return Result{Stdout: stdout.String(), Stderr: stderr.String()}, nil
}

func redactCommandArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}

	out := make([]string, len(args))
	redactNext := false
	for i, arg := range args {
		if redactNext {
			out[i] = "<redacted>"
			redactNext = false
			continue
		}

		lower := strings.ToLower(strings.TrimSpace(arg))
		if lower == "" {
			out[i] = arg
			continue
		}

		if key, _, ok := strings.Cut(lower, "="); ok {
			if isSensitiveArgKey(key) {
				prefixLen := len(arg) - len(strings.TrimPrefix(lower, key+"="))
				if prefixLen < 0 || prefixLen > len(arg) {
					out[i] = key + "=<redacted>"
				} else {
					out[i] = arg[:prefixLen] + "<redacted>"
				}
				continue
			}
		}

		if isSensitiveArgKey(lower) {
			out[i] = arg
			redactNext = true
			continue
		}

		if strings.HasPrefix(lower, "-p") && len(arg) > 2 {
			out[i] = arg[:2] + "<redacted>"
			continue
		}

		out[i] = arg
	}
	return out
}

func isSensitiveArgKey(key string) bool {
	switch strings.TrimSpace(strings.ToLower(key)) {
	case "--password", "--admin_password", "--dbpass", "--db-password", "--db_password", "--secret", "--token", "--api-key", "--apikey":
		return true
	default:
		return false
	}
}
