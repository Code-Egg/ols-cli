package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ols/ols-cli/internal/service"
)

type captureCLIUpdater struct {
	opts service.CLIUpdateOptions
	err  error
}

func (c *captureCLIUpdater) UpdateCLI(_ context.Context, opts service.CLIUpdateOptions) error {
	c.opts = opts
	return c.err
}

func TestUpdateCmdPassesDryRunOption(t *testing.T) {
	updater := &captureCLIUpdater{}
	rootOpts := &rootOptions{DryRun: true}
	cmd := newUpdateCmd(updater, rootOpts)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}
	if !updater.opts.DryRun {
		t.Fatal("expected dry-run option to be forwarded")
	}
}

func TestUpdateCmdWrapsServiceError(t *testing.T) {
	updater := &captureCLIUpdater{err: errors.New("boom")}
	cmd := newUpdateCmd(updater, &rootOptions{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected execute error")
	}
	if !strings.Contains(err.Error(), "update failed") {
		t.Fatalf("expected wrapped update error, got: %v", err)
	}
}
