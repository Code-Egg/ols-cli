package service

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
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
	defaultLSWSRoot = "/usr/local/lsws"
	defaultWebRoot  = "/var/www"
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
	PHPVersion string
	DryRun     bool
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

	pkgs := packagesForInstall(phpVersion)

	s.console.Section("Install runtime")
	s.console.Bullet("Platform: " + info.Summary())
	s.console.Bullet("PHP: lsphp" + phpVersion)
	for _, p := range pkgs {
		s.console.Bullet("Package: " + p)
	}

	if opts.DryRun {
		s.console.Warn("Dry-run enabled: no system changes were made")
		s.console.Bullet("configure LiteSpeed package repository")
		s.console.Bullet("install runtime packages")
		s.console.Success("Dry-run plan generated")
		return nil
	}

	if err := s.configureLiteSpeedRepo(ctx, info); err != nil {
		return err
	}

	installer := platform.NewPackageInstaller(s.runner, info)
	if err := installer.Install(ctx, pkgs...); err != nil {
		return err
	}

	s.console.Success("OpenLiteSpeed runtime installed")
	s.console.Bullet("Binary: " + filepath.Join(s.lswsRoot, "bin", "lswsctrl"))
	return nil
}

func (s SiteService) CreateSite(ctx context.Context, opts CreateSiteOptions) error {
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

	siteRoot := filepath.Join(s.webRoot, opts.Domain)
	docRoot := filepath.Join(siteRoot, "html")
	vhostDir := filepath.Join(s.lswsRoot, "conf", "vhosts", opts.Domain)
	vhostConfig := filepath.Join(vhostDir, "vhconf.conf")
	vhostDefinition := filepath.Join(vhostDir, "vhost.conf")

	s.console.Section("Create site")
	s.console.Bullet("Domain: " + opts.Domain)
	s.console.Bullet("PHP: lsphp" + phpVersion)
	s.console.Bullet("Platform: " + info.Summary())
	s.console.Bullet("Document root: " + docRoot)
	s.console.Bullet("VHost config: " + vhostConfig)
	if opts.WithWordPress {
		s.console.Bullet("WordPress: enabled")
	}
	if opts.WithLE {
		s.console.Bullet("Let's Encrypt: enabled")
	}

	if opts.DryRun {
		s.console.Warn("Dry-run enabled: no system changes were made")
		s.console.Info("Planned filesystem operations:")
		s.console.Bullet("mkdir -p " + docRoot)
		s.console.Bullet("mkdir -p " + vhostDir)
		s.console.Bullet("write " + vhostConfig)
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

	s.console.Success("Virtual host files created")
	s.console.Bullet("Virtual host definition: " + vhostDefinition)
	s.console.Bullet("Virtual host config: " + vhostConfig)
	s.console.Bullet("Document root: " + docRoot)
	s.console.Warn("Map this vhost in OpenLiteSpeed listener (WebAdmin) and reload OLS")
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

func (s SiteService) configureLiteSpeedRepo(ctx context.Context, info platform.Info) error {
	switch info.PackageManager {
	case platform.PackageManagerAPT, platform.PackageManagerYUM, platform.PackageManagerDNF:
		setup := "if command -v wget >/dev/null 2>&1; then wget -qO - https://repo.litespeed.sh | bash; " +
			"elif command -v curl >/dev/null 2>&1; then curl -fsSL https://repo.litespeed.sh | bash; " +
			"else echo 'wget or curl is required to add LiteSpeed repo' >&2; exit 1; fi"
		if _, err := s.runner.Run(ctx, "bash", "-c", setup); err != nil {
			return apperr.Wrap(apperr.CodeCommand, "failed to configure LiteSpeed package repository", err)
		}
		return nil
	default:
		return apperr.New(apperr.CodePlatform, fmt.Sprintf("unsupported package manager: %s", info.PackageManager))
	}
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

func fileExists(p string) bool {
	if strings.TrimSpace(p) == "" {
		return false
	}
	_, err := os.Stat(p)
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

func installWordPress(docRoot string) error {
	const wpArchiveURL = "https://wordpress.org/latest.tar.gz"

	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(wpArchiveURL)
	if err != nil {
		return apperr.Wrap(apperr.CodeCommand, "failed to download WordPress archive", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return apperr.New(apperr.CodeCommand, fmt.Sprintf("failed to download WordPress archive: http %d", resp.StatusCode))
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return apperr.Wrap(apperr.CodeCommand, "failed to open WordPress archive", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	root := filepath.Clean(docRoot)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return apperr.Wrap(apperr.CodeCommand, "failed to read WordPress archive entry", err)
		}

		cleanName := path.Clean(hdr.Name)
		if cleanName == "." || cleanName == "wordpress" {
			continue
		}
		if !strings.HasPrefix(cleanName, "wordpress/") {
			continue
		}

		rel := strings.TrimPrefix(cleanName, "wordpress/")
		if rel == "" {
			continue
		}

		target := filepath.Join(root, filepath.FromSlash(rel))
		cleanTarget := filepath.Clean(target)
		if cleanTarget != root && !strings.HasPrefix(cleanTarget, root+string(os.PathSeparator)) {
			return apperr.New(apperr.CodeConfig, "unsafe WordPress archive path detected")
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(cleanTarget, 0o755); err != nil {
				return apperr.Wrap(apperr.CodeConfig, "failed to create WordPress directory", err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(cleanTarget), 0o755); err != nil {
				return apperr.Wrap(apperr.CodeConfig, "failed to create WordPress file directory", err)
			}
			mode := os.FileMode(hdr.Mode)
			if mode == 0 {
				mode = 0o644
			}
			f, err := os.OpenFile(cleanTarget, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
			if err != nil {
				return apperr.Wrap(apperr.CodeConfig, "failed to create WordPress file", err)
			}
			if _, err := io.Copy(f, tr); err != nil {
				_ = f.Close()
				return apperr.Wrap(apperr.CodeConfig, "failed to write WordPress file", err)
			}
			if err := f.Close(); err != nil {
				return apperr.Wrap(apperr.CodeConfig, "failed to finalize WordPress file", err)
			}
		default:
			// Skip non-regular entries for safety.
		}
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
