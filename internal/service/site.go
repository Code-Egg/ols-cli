package service

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ols/ols-cli/internal/apperr"
	"github.com/ols/ols-cli/internal/platform"
	"github.com/ols/ols-cli/internal/runner"
	"github.com/ols/ols-cli/internal/ui"
)

var domainPattern = regexp.MustCompile(`(?i)^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)

const (
	defaultLSWSRoot       = "/usr/local/lsws"
	defaultWebRoot        = "/var/www"
	liteSpeedRepoSetupCmd = "wget -O - https://repo.litespeed.sh | bash"
	wordpressTarballURL   = "https://wordpress.org/latest.tar.gz"
)

// SiteService contains site lifecycle workflows.
type SiteService struct {
	detector platform.Detector
	runner   runner.Runner
	console  ui.Console
	lswsRoot string
	webRoot  string
}

func NewSiteService(detector platform.Detector, run runner.Runner, console ui.Console) SiteService {
	return NewSiteServiceWithPaths(detector, run, console, defaultLSWSRoot, defaultWebRoot)
}

func NewSiteServiceWithPaths(detector platform.Detector, run runner.Runner, console ui.Console, lswsRoot, webRoot string) SiteService {
	lswsRoot = strings.TrimSpace(lswsRoot)
	if lswsRoot == "" {
		lswsRoot = defaultLSWSRoot
	}
	webRoot = strings.TrimSpace(webRoot)
	if webRoot == "" {
		webRoot = defaultWebRoot
	}
	return SiteService{detector: detector, runner: run, console: console, lswsRoot: lswsRoot, webRoot: webRoot}
}

type InstallOptions struct {
	PHPVersion    string
	WithWordPress bool
	WithLE        bool
	DryRun        bool
}

type CreateSiteOptions struct {
	Domain        string
	WithWordPress bool
	WithLE        bool
	PHPVersion    string
	DryRun        bool
}

type UpdateSiteOptions struct {
	Domain     string
	PHPVersion string
	DryRun     bool
}

func (s SiteService) InstallRuntime(ctx context.Context, opts InstallOptions) error {
	phpVersion, err := NormalizePHPVersion(opts.PHPVersion)
	if err != nil {
		return err
	}

	info, err := s.detector.Detect(ctx)
	if err != nil {
		return err
	}

	s.console.Section("Install runtime")
	s.console.Bullet("PHP: lsphp" + phpVersion)
	s.console.Bullet("Platform: " + info.Summary())
	if opts.WithWordPress {
		s.console.Bullet("WordPress dependencies: enabled")
	}
	if opts.WithLE {
		s.console.Bullet("Let's Encrypt dependencies: enabled")
	}

	packages := packagesForInstall(info, phpVersion, opts.WithWordPress, opts.WithLE)

	if opts.DryRun {
		s.console.Warn("Dry-run enabled: no system changes were made")
		s.console.Info("Planned repository setup:")
		s.console.Bullet("write " + vhostDefinition)
		if opts.WithWordPress {
			s.console.Bullet("download and extract WordPress into " + docRoot)
		} else {
			s.console.Bullet("write starter index.php into " + docRoot)
		}
		s.console.Success("Dry-run plan generated")
		return nil
	}

	if err := s.ensureRuntimeInstalled(phpVersion); err != nil {
		return err
	}

	if err := os.MkdirAll(docRoot, 0o755); err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to create document root", err)
	}
	if err := os.MkdirAll(vhostDir, 0o755); err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to create virtual host directory", err)
	}

	if err := os.WriteFile(vhostConfig, []byte(buildVHConfig(phpVersion)), 0o644); err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to write vhost config", err)
	}

	if err := os.WriteFile(vhostDefinition, []byte(buildVHostDefinition(opts.Domain, siteRoot, vhostConfig)), 0o644); err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to write vhost definition", err)
	}

	if opts.WithWordPress {
		if err := installWordPress(docRoot); err != nil {
			return err
		}
		s.console.Success("WordPress files provisioned")
	} else {
		if err := ensureStarterIndex(docRoot, opts.Domain); err != nil {
			return err
		}
	}

	if opts.WithLE {
		s.console.Warn("SSL issuance is not yet automated; run certbot and wire SSL listener manually")
	}

	s.console.Success("Virtual host scaffold created")
	s.console.Bullet("Virtual host definition: " + vhostDefinition)
	s.console.Bullet("Virtual host config: " + vhostConfig)
	s.console.Bullet("Document root: " + docRoot)
	s.console.Warn("Map this vhost in OpenLiteSpeed listener (WebAdmin) if not already mapped")
	return nil
}

func (s SiteService) UpdateSitePHP(ctx context.Context, opts UpdateSiteOptions) error {
	if err := ValidateDomain(opts.Domain); err != nil {
		return err
	}

	phpVersion, err := NormalizePHPVersion(opts.PHPVersion)
	if err != nil {
		return err
	}

	info, err := s.detector.Detect(ctx)
	if err != nil {
		return err
	}

	s.console.Section("Update site PHP")
	s.console.Bullet("Domain: " + opts.Domain)
	s.console.Bullet("Target: lsphp" + phpVersion)
	s.console.Bullet("Platform: " + info.Summary())

	packages := packagesForPHPUpdate(phpVersion)
	if opts.DryRun {
		s.console.Warn("Dry-run enabled: no system changes were made")
		s.console.Info("Planned package install:")
		for _, pkg := range packages {
			s.console.Bullet(pkg)
		}
		s.console.Success("Dry-run plan generated")
		return nil
	}

	installer := platform.NewPackageInstaller(s.runner, info)
	if err := installer.Install(ctx, packages...); err != nil {
		return err
	}

	s.console.Success("Requested PHP package installed")
	s.console.Warn("OpenLiteSpeed vhost PHP handler switch is scaffolded next step")
	return nil
}

func (s SiteService) ensureRuntimeInstalled(phpVersion string) error {
	lswsCtrlPath := filepath.Join(s.lswsRoot, "bin", "lswsctrl")
	if !fileExists(lswsCtrlPath) {
		return apperr.New(
			apperr.CodeValidation,
			fmt.Sprintf("OpenLiteSpeed runtime was not found at %s; run `ols install --php%s` first", lswsCtrlPath, phpVersion),
		)
	}

	phpBinary := filepath.Join(s.lswsRoot, "lsphp"+phpVersion, "bin", "lsphp")
	if !fileExists(phpBinary) {
		return apperr.New(
			apperr.CodeValidation,
			fmt.Sprintf("PHP runtime lsphp%s was not found at %s; run `ols install --php%s` first", phpVersion, phpBinary, phpVersion),
		)
	}

	return nil
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func ensureStarterIndex(docRoot, domain string) error {
	indexPath := filepath.Join(docRoot, "index.php")
	if fileExists(indexPath) {
		return nil
	}
	content := fmt.Sprintf("<?php\nheader('Content-Type: text/plain; charset=utf-8');\necho \"Site %s is ready.\\n\";\n", domain)
	if err := os.WriteFile(indexPath, []byte(content), 0o644); err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to write starter index.php", err)
	}
	return nil
}

func buildVHostDefinition(domain, siteRoot, vhostConfigPath string) string {
	return fmt.Sprintf(
		"virtualhost %s {\n  vhRoot                  %s/\n  configFile              %s\n  allowSymbolLink         1\n  enableScript            1\n  restrained              1\n}\n",
		domain,
		siteRoot,
		vhostConfigPath,
	)
}

func buildVHConfig(phpVersion string) string {
	return fmt.Sprintf("docRoot                   $VH_ROOT/html/\n\nindex  {\n  useServer               0\n  indexFiles              index.php, index.html\n}\n\ncontext / {\n  type                    null\n  location                $DOC_ROOT/\n  allowBrowse             1\n}\n\nextprocessor lsphp%s {\n  type                    lsapi\n  address                 uds://tmp/lshttpd/lsphp%s.sock\n  maxConns                35\n  env                     PHP_LSAPI_CHILDREN=35\n  initTimeout             60\n  retryTimeout            0\n  persistConn             1\n  pcKeepAliveTimeout      1\n  respBuffer              0\n  autoStart               2\n  path                    /usr/local/lsws/lsphp%s/bin/lsphp\n  backlog                 100\n  instances               1\n  priority                0\n  memSoftLimit            2047M\n  memHardLimit            2047M\n  procSoftLimit           400\n  procHardLimit           500\n}\n\nscriptHandler  {\n  add                     lsapi:lsphp%s php\n}\n\nrewrite  {\n  enable                  1\n  autoLoadHtaccess        1\n}\n", phpVersion, phpVersion, phpVersion, phpVersion)
}

func NormalizePHPVersion(in string) (string, error) {
	candidate := strings.TrimSpace(strings.ToLower(in))
	candidate = strings.TrimPrefix(candidate, "lsphp")
	candidate = strings.ReplaceAll(candidate, ".", "")

	supported := map[string]struct{}{
		"74": {},
		"80": {},
		"81": {},
		"82": {},
		"83": {},
		"84": {},
	}

	if _, ok := supported[candidate]; !ok {
		return "", apperr.New(
			apperr.CodeValidation,
			fmt.Sprintf("unsupported PHP version: %q (allowed: 74,80,81,82,83,84)", in),
		)
	}

	return candidate, nil
}

func ValidateDomain(domain string) error {
	d := strings.TrimSpace(strings.ToLower(domain))
	if d == "" {
		return apperr.New(apperr.CodeValidation, "domain is required")
	}
	if len(d) > 253 {
		return apperr.New(apperr.CodeValidation, "domain exceeds 253 characters")
	}
	if !domainPattern.MatchString(d) {
		return apperr.New(apperr.CodeValidation, "invalid domain format")
	}
	return nil
}

func packagesForInstall(phpVersion string) []string {
	return []string{"openlitespeed", "lsphp" + phpVersion, "lsphp" + phpVersion + "-mysql"}
}

func packagesForPHPUpdate(phpVersion string) []string {
	return []string{"lsphp" + phpVersion, "lsphp" + phpVersion + "-mysql"}
}
