package cli

import (
	"context"
	"fmt"

	"github.com/ols/ols-cli/internal/service"
	"github.com/spf13/cobra"
)

type cliUpdater interface {
	UpdateCLI(ctx context.Context, opts service.CLIUpdateOptions) error
}

func newUpdateCmd(svc cliUpdater, rootOpts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update",
		Short:   "Update ols to the latest GitHub release binary",
		Example: "ols update\nols --dry-run update",
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := svc.UpdateCLI(cmd.Context(), service.CLIUpdateOptions{DryRun: rootOpts.DryRun})
			if err != nil {
				return fmt.Errorf("update failed: %w", err)
			}
			return nil
		},
	}
	return cmd
}
