package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/ols/ols-cli/internal/apperr"
	"github.com/ols/ols-cli/internal/platform"
	"github.com/ols/ols-cli/internal/runner"
	"github.com/ols/ols-cli/internal/service"
	"github.com/ols/ols-cli/internal/ui"
	"github.com/spf13/cobra"
)

type rootOptions struct {
	DryRun bool
	Color  string
}

func NewRootCmd() *cobra.Command {
	cobra.EnableCommandSorting = false

	opts := &rootOptions{Color: "always"}
	_ = configureColorOutput(opts.Color)
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
			if err := configureColorOutput(opts.Color); err != nil {
				return err
			}
			if !hasRootPrivileges() && !opts.DryRun {
				return apperr.New(apperr.CodeValidation, "root privileges are required (use sudo), or pass --dry-run")
			}
			return nil
		},
	}

	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.PersistentFlags().BoolVar(&opts.DryRun, "dry-run", false, "print planned operations without changing the system")
	cmd.PersistentFlags().StringVar(&opts.Color, "color", "always", "color mode: always, auto, never")
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return apperr.Wrap(apperr.CodeValidation, "invalid command options", err)
	})

	cmd.AddCommand(newInstallCmd(svc, opts))
	cmd.AddCommand(newSiteCmd(svc, opts))
	cmd.AddCommand(newUpdateCmd(svc, opts))
	cmd.SetHelpCommand(newRootHelpCmd(cmd))
	applyColorHelpTemplate(cmd)
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

func configureColorOutput(mode string) error {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "always":
		_ = os.Unsetenv("NO_COLOR")
		_ = os.Setenv("CLICOLOR", "1")
		_ = os.Setenv("CLICOLOR_FORCE", "1")
		if strings.TrimSpace(os.Getenv("TERM")) == "" {
			_ = os.Setenv("TERM", "xterm-256color")
		}
		lipgloss.SetColorProfile(termenv.TrueColor)
		return nil
	case "auto":
		_ = os.Unsetenv("CLICOLOR_FORCE")
		lipgloss.SetColorProfile(termenv.ColorProfile())
		return nil
	case "never":
		_ = os.Setenv("NO_COLOR", "1")
		_ = os.Setenv("CLICOLOR", "0")
		_ = os.Unsetenv("CLICOLOR_FORCE")
		lipgloss.SetColorProfile(termenv.Ascii)
		return nil
	default:
		return apperr.New(apperr.CodeValidation, "invalid --color value; allowed: always, auto, never")
	}
}

func applyColorHelpTemplate(cmd *cobra.Command) {
	cmd.SetHelpTemplate(colorizeHelpTemplate(cmd.HelpTemplate()))
	cmd.SetUsageTemplate(colorizeHelpTemplate(cmd.UsageTemplate()))
	for _, c := range cmd.Commands() {
		applyColorHelpTemplate(c)
	}
}

func colorizeHelpTemplate(template string) string {
	const (
		boldCyan   = "\x1b[1;36m"
		boldGreen  = "\x1b[1;32m"
		boldBlue   = "\x1b[1;34m"
		boldYellow = "\x1b[1;33m"
		dimWhite   = "\x1b[2;37m"
		reset      = "\x1b[0m"
	)
	return strings.NewReplacer(
		"Usage:", "\n"+boldCyan+"USAGE"+reset+"\n"+dimWhite+"─────"+reset,
		"Aliases:", "\n"+boldGreen+"ALIASES"+reset+"\n"+dimWhite+"───────"+reset,
		"Examples:", "\n"+boldYellow+"EXAMPLES"+reset+"\n"+dimWhite+"────────"+reset,
		"Available Commands:", "\n"+boldGreen+"AVAILABLE COMMANDS"+reset+"\n"+dimWhite+"──────────────────"+reset,
		"Flags:", "\n"+boldBlue+"FLAGS"+reset+"\n"+dimWhite+"─────"+reset,
		"Global Flags:", "\n"+boldBlue+"GLOBAL FLAGS"+reset+"\n"+dimWhite+"────────────"+reset,
		"Additional help topics:", "\n"+boldGreen+"ADDITIONAL HELP TOPICS"+reset+"\n"+dimWhite+"──────────────────────"+reset,
	).Replace(template)
}

func newRootHelpCmd(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:   "help [command]",
		Short: "Help about any command",
		Long:  "Help provides help for any command in the application.",
		Args:  cobra.MaximumNArgs(1),
		Run: func(_ *cobra.Command, args []string) {
			if len(args) == 0 {
				_ = root.Help()
				return
			}
			target, _, err := root.Find(args)
			if err != nil || target == nil {
				_ = root.Help()
				return
			}
			_ = target.Help()
		},
	}
}
