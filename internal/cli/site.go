package cli

import (
	"context"
	"fmt"

	"github.com/ols/ols-cli/internal/apperr"
	"github.com/ols/ols-cli/internal/service"
	"github.com/spf13/cobra"
)

type siteManager interface {
	CreateSite(ctx context.Context, opts service.CreateSiteOptions) error
	UpdateSitePHP(ctx context.Context, opts service.UpdateSiteOptions) error
}

type phpFlags struct {
	v74 bool
	v80 bool
	v81 bool
	v82 bool
	v83 bool
	v84 bool
}

func (f phpFlags) selected(defaultVersion string) (string, error) {
	selected := ""
	count := 0

	if f.v74 {
		selected = "74"
		count++
	}
	if f.v80 {
		selected = "80"
		count++
	}
	if f.v81 {
		selected = "81"
		count++
	}
	if f.v82 {
		selected = "82"
		count++
	}
	if f.v83 {
		selected = "83"
		count++
	}
	if f.v84 {
		selected = "84"
		count++
	}

	if count > 1 {
		return "", apperr.New(apperr.CodeValidation, "multiple PHP version flags provided; choose only one of --php74/--php80/--php81/--php82/--php83/--php84")
	}
	if count == 0 {
		return defaultVersion, nil
	}
	return selected, nil
}

func addPHPVersionFlags(cmd *cobra.Command, flags *phpFlags) {
	cmd.Flags().BoolVar(&flags.v74, "php74", false, "use PHP 7.4")
	cmd.Flags().BoolVar(&flags.v80, "php80", false, "use PHP 8.0")
	cmd.Flags().BoolVar(&flags.v81, "php81", false, "use PHP 8.1")
	cmd.Flags().BoolVar(&flags.v82, "php82", false, "use PHP 8.2")
	cmd.Flags().BoolVar(&flags.v83, "php83", false, "use PHP 8.3")
	cmd.Flags().BoolVar(&flags.v84, "php84", false, "use PHP 8.4")
}

func newSiteCmd(svc siteManager, rootOpts *rootOptions) *cobra.Command {
	siteCmd := &cobra.Command{
		Use:   "site",
		Short: "Manage OpenLiteSpeed sites",
	}

	siteCmd.AddCommand(newSiteCreateCmd(svc, rootOpts))
	siteCmd.AddCommand(newSiteUpdateCmd(svc, rootOpts))
	return siteCmd
}

func newSiteCreateCmd(svc siteManager, rootOpts *rootOptions) *cobra.Command {
	php := &phpFlags{}
	var withWordPress bool
	var withLE bool

	cmd := &cobra.Command{
		Use:   "create <domain>",
		Short: "Create a new OpenLiteSpeed virtual host",
		Example: "ols site create example.com --wp\n" +
			"ols site create example.com --wp --php84\n" +
			"ols --dry-run site create example.com --wp --le --php82",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			phpVersion, err := php.selected("82")
			if err != nil {
				return err
			}
			err = svc.CreateSite(cmd.Context(), service.CreateSiteOptions{
				Domain:        args[0],
				WithWordPress: withWordPress,
				WithLE:        withLE,
				PHPVersion:    phpVersion,
				DryRun:        rootOpts.DryRun,
			})
			if err != nil {
				return fmt.Errorf("site create failed: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&withWordPress, "wp", false, "install WordPress with required dependencies")
	cmd.Flags().BoolVar(&withLE, "le", false, "configure Let's Encrypt certificate")
	addPHPVersionFlags(cmd, php)
	return cmd
}

func newSiteUpdateCmd(svc siteManager, rootOpts *rootOptions) *cobra.Command {
	php := &phpFlags{}
	var withWordPress bool

	cmd := &cobra.Command{
		Use:   "update <domain>",
		Short: "Update existing site configuration",
		Example: "ols site update example.com --php83\n" +
			"ols site update example.com --wp --php84\n" +
			"ols --dry-run site update example.com --wp --php82",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			phpVersion, err := php.selected("")
			if err != nil {
				return err
			}
			if phpVersion == "" {
				return apperr.New(apperr.CodeValidation, "missing PHP version flag; provide one of --php74/--php80/--php81/--php82/--php83/--php84")
			}
			err = svc.UpdateSitePHP(cmd.Context(), service.UpdateSiteOptions{
				Domain:        args[0],
				WithWordPress: withWordPress,
				PHPVersion:    phpVersion,
				DryRun:        rootOpts.DryRun,
			})
			if err != nil {
				return fmt.Errorf("site update failed: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&withWordPress, "wp", false, "ensure WordPress and LiteSpeed Cache plugin are present")
	addPHPVersionFlags(cmd, php)
	return cmd
}
