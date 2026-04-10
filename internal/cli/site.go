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
	SiteInfo(ctx context.Context, opts service.SiteInfoOptions) error
	ShowSiteConfig(ctx context.Context, opts service.ShowSiteConfigOptions) error
	ListSites(ctx context.Context, opts service.ListSitesOptions) error
	DeleteSite(ctx context.Context, opts service.DeleteSiteOptions) error
}

type phpFlags struct {
	v81 bool
	v82 bool
	v83 bool
	v84 bool
	v85 bool
}

type toggleFlags struct {
	enable  bool
	disable bool
}

func (f toggleFlags) selected(feature string) (*bool, error) {
	if f.enable && f.disable {
		return nil, apperr.New(apperr.CodeValidation, fmt.Sprintf("conflicting flags for %s; choose only one of --enable-%s/--disable-%s", feature, feature, feature))
	}
	if f.enable {
		enabled := true
		return &enabled, nil
	}
	if f.disable {
		disabled := false
		return &disabled, nil
	}
	return nil, nil
}

func (f phpFlags) selected(defaultVersion string) (string, error) {
	selected := ""
	count := 0

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
	if f.v85 {
		selected = "85"
		count++
	}

	if count > 1 {
		return "", apperr.New(apperr.CodeValidation, "multiple PHP version flags provided; choose only one of --php81/--php82/--php83/--php84/--php85")
	}
	if count == 0 {
		return defaultVersion, nil
	}
	return selected, nil
}

func addPHPVersionFlags(cmd *cobra.Command, flags *phpFlags) {
	cmd.Flags().BoolVar(&flags.v81, "php81", false, "use PHP 8.1")
	cmd.Flags().BoolVar(&flags.v82, "php82", false, "use PHP 8.2")
	cmd.Flags().BoolVar(&flags.v83, "php83", false, "use PHP 8.3")
	cmd.Flags().BoolVar(&flags.v84, "php84", false, "use PHP 8.4")
	cmd.Flags().BoolVar(&flags.v85, "php85", false, "use PHP 8.5")
}

func newSiteCmd(svc siteManager, rootOpts *rootOptions) *cobra.Command {
	siteCmd := &cobra.Command{
		Use:   "site",
		Short: "Manage OpenLiteSpeed sites",
	}

	siteCmd.AddCommand(newSiteCreateCmd(svc, rootOpts))
	siteCmd.AddCommand(newSiteUpdateCmd(svc, rootOpts))
	siteCmd.AddCommand(newSiteInfoCmd(svc, rootOpts))
	siteCmd.AddCommand(newSiteShowCmd(svc, rootOpts))
	siteCmd.AddCommand(newSiteListCmd(svc, rootOpts))
	siteCmd.AddCommand(newSiteDeleteCmd(svc, rootOpts))
	return siteCmd
}

func newSiteCreateCmd(svc siteManager, rootOpts *rootOptions) *cobra.Command {
	php := &phpFlags{}
	owasp := &toggleFlags{}
	recaptcha := &toggleFlags{}
	var withWordPress bool
	var withLE bool
	var withHSTS bool
	namespace := &toggleFlags{}

	cmd := &cobra.Command{
		Use:   "create <domain>",
		Short: "Create a new OpenLiteSpeed virtual host",
		Example: "ols site create example.com --wp\n" +
			"ols site create example.com --wp --php84 --enable-owasp --enable-recaptcha\n" +
			"ols --dry-run site create example.com --wp --le --php85 --hsts",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			phpVersion, err := php.selected("85")
			if err != nil {
				return err
			}
			owaspEnabled, err := owasp.selected("owasp")
			if err != nil {
				return err
			}
			recaptchaEnabled, err := recaptcha.selected("recaptcha")
			if err != nil {
				return err
			}
			namespaceEnabled, err := namespace.selected("ns")
			if err != nil {
				return err
			}
			err = svc.CreateSite(cmd.Context(), service.CreateSiteOptions{
				Domain:            args[0],
				WithWordPress:     withWordPress,
				WithLE:            withLE,
				PHPVersion:        phpVersion,
				OWASPEnabled:      owaspEnabled,
				RecaptchaEnabled:  recaptchaEnabled,
				NamespaceEnabled:  namespaceEnabled,
				EnableHSTSHeaders: withHSTS,
				DryRun:            rootOpts.DryRun,
			})
			if err != nil {
				return fmt.Errorf("site create failed: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().SortFlags = false
	cmd.Flags().BoolVar(&withWordPress, "wp", false, "install WordPress with required dependencies")
	cmd.Flags().BoolVar(&withLE, "le", false, "configure Let's Encrypt certificate")
	addPHPVersionFlags(cmd, php)
	cmd.Flags().BoolVar(&owasp.enable, "enable-owasp", false, "enable OWASP ModSecurity at virtual host level")
	cmd.Flags().BoolVar(&recaptcha.enable, "enable-recaptcha", false, "enable reCAPTCHA at virtual host level")
	cmd.Flags().BoolVar(&owasp.disable, "disable-owasp", false, "disable OWASP ModSecurity at virtual host level")
	cmd.Flags().BoolVar(&recaptcha.disable, "disable-recaptcha", false, "disable reCAPTCHA at virtual host level")
	cmd.Flags().BoolVar(&namespace.enable, "enable-ns", false, "enable namespace at virtual host level")
	cmd.Flags().BoolVar(&namespace.disable, "disable-ns", false, "disable namespace at virtual host level")
	cmd.Flags().BoolVar(&withHSTS, "hsts", false, "add recommended security extra headers to static context /")
	return cmd
}

func newSiteUpdateCmd(svc siteManager, rootOpts *rootOptions) *cobra.Command {
	php := &phpFlags{}
	owasp := &toggleFlags{}
	recaptcha := &toggleFlags{}
	var withWordPress bool
	var withHSTS bool
	namespace := &toggleFlags{}

	cmd := &cobra.Command{
		Use:   "update <domain>",
		Short: "Update existing site configuration",
		Example: "ols site update example.com --php83\n" +
			"ols site update example.com --enable-owasp --enable-recaptcha\n" +
			"ols --dry-run site update example.com --wp --php85 --hsts",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			phpVersion, err := php.selected("")
			if err != nil {
				return err
			}
			owaspEnabled, err := owasp.selected("owasp")
			if err != nil {
				return err
			}
			recaptchaEnabled, err := recaptcha.selected("recaptcha")
			if err != nil {
				return err
			}

			namespaceEnabled, err := namespace.selected("ns")
			if err != nil {
				return err
			}

			if withWordPress && phpVersion == "" {
				return apperr.New(apperr.CodeValidation, "missing PHP version flag for --wp; provide one of --php81/--php82/--php83/--php84/--php85")
			}
			if phpVersion == "" && !withWordPress && owaspEnabled == nil && recaptchaEnabled == nil && namespaceEnabled == nil && !withHSTS {
				return apperr.New(apperr.CodeValidation, "no update action provided; pass PHP version and/or security flags such as --enable-owasp, --enable-recaptcha, --disable-owasp, --disable-recaptcha, --enable-ns, --disable-ns, --hsts")
			}
			err = svc.UpdateSitePHP(cmd.Context(), service.UpdateSiteOptions{
				Domain:            args[0],
				WithWordPress:     withWordPress,
				PHPVersion:        phpVersion,
				OWASPEnabled:      owaspEnabled,
				RecaptchaEnabled:  recaptchaEnabled,
				NamespaceEnabled:  namespaceEnabled,
				EnableHSTSHeaders: withHSTS,
				DryRun:            rootOpts.DryRun,
			})
			if err != nil {
				return fmt.Errorf("site update failed: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().SortFlags = false
	cmd.Flags().BoolVar(&withWordPress, "wp", false, "ensure WordPress and LiteSpeed Cache plugin are present")
	addPHPVersionFlags(cmd, php)
	cmd.Flags().BoolVar(&owasp.enable, "enable-owasp", false, "enable OWASP ModSecurity at virtual host level")
	cmd.Flags().BoolVar(&recaptcha.enable, "enable-recaptcha", false, "enable reCAPTCHA at virtual host level")
	cmd.Flags().BoolVar(&owasp.disable, "disable-owasp", false, "disable OWASP ModSecurity at virtual host level")
	cmd.Flags().BoolVar(&recaptcha.disable, "disable-recaptcha", false, "disable reCAPTCHA at virtual host level")
	cmd.Flags().BoolVar(&namespace.enable, "enable-ns", false, "enable namespace at virtual host level")
	cmd.Flags().BoolVar(&namespace.disable, "disable-ns", false, "disable namespace at virtual host level")
	cmd.Flags().BoolVar(&withHSTS, "hsts", false, "add recommended security extra headers to static context /")
	return cmd
}

func newSiteInfoCmd(svc siteManager, rootOpts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "info <domain>",
		Short:   "Show site information",
		Example: "ols site info example.com",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			err := svc.SiteInfo(cmd.Context(), service.SiteInfoOptions{
				Domain: args[0],
				DryRun: rootOpts.DryRun,
			})
			if err != nil {
				return fmt.Errorf("site info failed: %w", err)
			}
			return nil
		},
	}
	return cmd
}

func newSiteShowCmd(svc siteManager, rootOpts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "show <domain>",
		Short:   "Display OpenLiteSpeed virtual host config",
		Example: "ols site show example.com",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			err := svc.ShowSiteConfig(cmd.Context(), service.ShowSiteConfigOptions{
				Domain: args[0],
				DryRun: rootOpts.DryRun,
			})
			if err != nil {
				return fmt.Errorf("site show failed: %w", err)
			}
			return nil
		},
	}
	return cmd
}

func newSiteListCmd(svc siteManager, rootOpts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List all configured sites",
		Example: "ols site list",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := svc.ListSites(cmd.Context(), service.ListSitesOptions{DryRun: rootOpts.DryRun})
			if err != nil {
				return fmt.Errorf("site list failed: %w", err)
			}
			return nil
		},
	}
	return cmd
}

func newSiteDeleteCmd(svc siteManager, rootOpts *rootOptions) *cobra.Command {
	var keepDatabase bool

	cmd := &cobra.Command{
		Use:   "delete <domain>",
		Short: "Delete site config, document root, and database",
		Example: "ols site delete example.com\n" +
			"ols --dry-run site delete example.com\n" +
			"ols site delete example.com --keep-db",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			err := svc.DeleteSite(cmd.Context(), service.DeleteSiteOptions{
				Domain:       args[0],
				DropDatabase: !keepDatabase,
				DryRun:       rootOpts.DryRun,
			})
			if err != nil {
				return fmt.Errorf("site delete failed: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&keepDatabase, "keep-db", false, "keep WordPress database and DB user")
	return cmd
}
