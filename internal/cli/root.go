package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/eric7/ols-cli/internal/apperr"
	"github.com/eric7/ols-cli/internal/platform"
	"github.com/eric7/ols-cli/internal/runner"
	"github.com/eric7/ols-cli/internal/service"
	"github.com/eric7/ols-cli/internal/ui"
	"github.com/spf13/cobra"
)

type rootOptions struct {
	DryRun bool
}

func NewRootCmd() *cobra.Command {
	opts := &rootOptions{}
	console := ui.NewStyledConsole(os.Stdout)
	detector := platform.NewOSReleaseDetector("")
	run := runner.NewExecRunner()
	svc := service.NewSiteService(detector, run, console)

	cmd := &cobra.Command{
		Use:           "ols",
		Short:         "OpenLiteSpeed site lifecycle CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       "0.1.0-dev",
		RunE: func(c *cobra.Command, _ []string) error {
			return c.Help()
		},
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			if !hasRootPrivileges() && !opts.DryRun {
				return apperr.New(apperr.CodeValidation, "root privileges are required (use sudo), or pass --dry-run")
			}
			return nil
		},
	}

	cmd.PersistentFlags().BoolVar(&opts.DryRun, "dry-run", false, "print planned operations without changing the system")
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return apperr.Wrap(apperr.CodeValidation, "invalid command options", err)
	})

	cmd.AddCommand(newSiteCmd(svc, opts))
	return wrapErrors(cmd)
}

func wrapErrors(cmd *cobra.Command) *cobra.Command {
	orig := cmd.RunE
	if orig != nil {
		cmd.RunE = func(c *cobra.Command, args []string) error {
			return renderError(orig(c, args))
		}
	}
	for _, child := range cmd.Commands() {
		wrapRunE(child)
	}
	return cmd
}

func wrapRunE(cmd *cobra.Command) {
	orig := cmd.RunE
	if orig != nil {
		cmd.RunE = func(c *cobra.Command, args []string) error {
			return renderError(orig(c, args))
		}
	}
	for _, child := range cmd.Commands() {
		wrapRunE(child)
	}
}

func renderError(err error) error {
	if err == nil {
		return nil
	}
	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		return fmt.Errorf("%s", appErr.Error())
	}
	return apperr.Wrap(apperr.CodeInternal, "unexpected failure", err)
}
