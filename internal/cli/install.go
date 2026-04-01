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
	var database string
	var configPath string
	var httpPort int
	var httpsPort int
	var sslCertFile string
	var sslKeyFile string
	var noListeners bool

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install OpenLiteSpeed runtime packages",
		RunE: func(cmd *cobra.Command, _ []string) error {
			phpVersion, err := php.selected("")
			if err != nil {
				return err
			}

			var configureListeners *bool
			if noListeners {
				disabled := false
				configureListeners = &disabled
			}

			err = svc.InstallRuntime(cmd.Context(), service.InstallOptions{
				PHPVersion:         phpVersion,
				DatabaseEngine:     database,
				ConfigPath:         configPath,
				ConfigureListeners: configureListeners,
				HTTPPort:           httpPort,
				HTTPSPort:          httpsPort,
				SSLCertFile:        sslCertFile,
				SSLKeyFile:         sslKeyFile,
				DryRun:             rootOpts.DryRun,
			})
			if err != nil {
				return fmt.Errorf("install failed: %w", err)
			}
			return nil
		},
	}

	addPHPVersionFlags(cmd, php)
	cmd.Flags().StringVar(&database, "database", "", "database engine override: mariadb|mysql|none")
	cmd.Flags().StringVar(&configPath, "config", "", "path to install config file (default: /etc/ols-cli/install.json)")
	cmd.Flags().IntVar(&httpPort, "http-port", 0, "HTTP listener port override (1-65535)")
	cmd.Flags().IntVar(&httpsPort, "https-port", 0, "HTTPS listener port override (1-65535)")
	cmd.Flags().StringVar(&sslCertFile, "ssl-cert", "", "SSL certificate file path override")
	cmd.Flags().StringVar(&sslKeyFile, "ssl-key", "", "SSL private key file path override")
	cmd.Flags().BoolVar(&noListeners, "no-listeners", false, "skip OpenLiteSpeed listener configuration")
	return cmd
}
