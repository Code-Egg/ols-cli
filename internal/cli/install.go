package cli

import (
	"context"
	"fmt"

	"github.com/ols/ols-cli/internal/service"
	"github.com/spf13/cobra"
)

type runtimeInstaller interface {
	InstallRuntime(ctx context.Context, opts service.InstallOptions) error
}

func newInstallCmd(svc runtimeInstaller, rootOpts *rootOptions) *cobra.Command {
	php := &phpFlags{}

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install OpenLiteSpeed runtime packages",
		RunE: func(cmd *cobra.Command, _ []string) error {
			phpVersion, err := php.selected("82")
			if err != nil {
				return err
			}

			err = svc.InstallRuntime(cmd.Context(), service.InstallOptions{
				PHPVersion: phpVersion,
				DryRun:     rootOpts.DryRun,
			})
			if err != nil {
				return fmt.Errorf("install failed: %w", err)
			}
			return nil
		},
	}

	addPHPVersionFlags(cmd, php)
	return cmd
}
