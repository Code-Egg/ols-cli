package service

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	crand "crypto/rand"
	"fmt"
	"io"
	"math/big"
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

var (
	domainPattern       = regexp.MustCompile(`(?i)^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
	lsphpNamePattern    = regexp.MustCompile(`lsphp[0-9]{2}`)
	nonAlphaNumPattern  = regexp.MustCompile(`[^a-z0-9]+`)
	multiUnderscoreExpr = regexp.MustCompile(`_+`)
)

const (
	defaultLSWSRoot      = "/usr/local/lsws"
	defaultWebRoot       = "/var/www"
	defaultSecretsRoot   = "/etc/ols-cli/sites"
	wpCLIPharURL         = "https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar"
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
	PHPVersion         string
	DatabaseEngine     string
	ConfigPath         string
	ConfigureListeners *bool
	HTTPPort           int
	HTTPSPort          int
	SSLCertFile        string
	SSLKeyFile         string
	DryRun             bool
}

type CreateSiteOptions struct {
	Domain        string
	WithWordPress bool
	WithLE        bool
	PHPVersion    string
	DryRun        bool
}

type UpdateSiteOptions struct {
	Domain        string
	WithWordPress bool
	PHPVersion    string
	DryRun        bool
}

func (s SiteService) InstallRuntime(ctx context.Context, opts InstallOptions) error {
	info, err := s.detector.Detect(ctx)
	if err != nil {
		return err
	}

	plan, err := resolveInstallPlan(opts, info, s.lswsRoot)
	if err != nil {
		return err
	}

	pkgs := packagesForInstall(plan.PHPVersion, plan.DatabasePackage)

	s.console.Section("Install runtime")
	s.console.Bullet("Platform: " + info.Summary())
	s.console.Bullet("Config: " + plan.ConfigPath)
	s.console.Bullet("PHP: lsphp" + plan.PHPVersion)
	s.console.Bullet("Database: " + plan.DatabaseEngine)
	for _, p := range pkgs {
		s.console.Bullet("Package: " + p)
	}

	if opts.DryRun {
		s.console.Warn("Dry-run enabled: no system changes were made")
		s.console.Bullet("configure LiteSpeed package repository")
		s.console.Bullet("install runtime packages")
		if plan.ConfigureListeners {
			s.console.Bullet(fmt.Sprintf("configure listeners in %s/conf/httpd_config.conf", s.lswsRoot))
			s.console.Bullet(fmt.Sprintf("HTTP listener port: %d", plan.HTTPPort))
			s.console.Bullet(fmt.Sprintf("HTTPS listener port: %d", plan.HTTPSPort))
			s.console.Bullet("SSL cert: " + plan.SSLCertFile)
			s.console.Bullet("SSL key: " + plan.SSLKeyFile)
		} else {
			s.console.Bullet("skip listener configuration")
		}
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

	if plan.ConfigureListeners {
		if err := s.configureRuntimeListeners(plan.HTTPPort, plan.HTTPSPort, plan.SSLCertFile, plan.SSLKeyFile); err != nil {
			return err
		}
	}

	s.console.Success("OpenLiteSpeed runtime installed")
	s.console.Bullet("Binary: " + filepath.Join(s.lswsRoot, "bin", "lswsctrl"))
	if plan.ConfigureListeners {
		s.console.Bullet(fmt.Sprintf("Listeners configured: HTTP %d / HTTPS %d", plan.HTTPPort, plan.HTTPSPort))
	}
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
	serverConfigPath := filepath.Join(s.lswsRoot, "conf", "httpd_config.conf")
	var wpAccess *wpAdminAccess

	s.console.Section("Create site")
	s.console.Bullet("Domain: " + opts.Domain)
	s.console.Bullet("PHP: lsphp" + phpVersion)
	s.console.Bullet("Platform: " + info.Summary())
	s.console.Bullet("Document root: " + docRoot)
	s.console.Bullet("VHost config: " + vhostConfig)
	s.console.Bullet("Server config: " + serverConfigPath)
	if opts.WithWordPress {
		s.console.Bullet("WordPress: enabled")
	}
	if opts.WithLE {
		s.console.Bullet("Let's Encrypt: enabled")
	}

	if opts.DryRun {
		s.console.Warn("Dry-run enabled: no system changes were made")
		s.console.Info("Planned filesystem operations:")
		s.console.Bullet("check duplicate domain in " + serverConfigPath)
		s.console.Bullet("mkdir -p " + docRoot)
		s.console.Bullet("mkdir -p " + vhostDir)
		s.console.Bullet("write " + vhostConfig)
		s.console.Bullet("write " + vhostDefinition)
		s.console.Bullet("append virtualhost block into " + serverConfigPath)
		s.console.Bullet("insert listener map in " + serverConfigPath)
		if opts.WithWordPress {
			s.console.Bullet("download and extract WordPress into " + docRoot)
			s.console.Bullet("download and install LiteSpeed Cache plugin")
			s.console.Bullet("create WordPress database and database user")
			s.console.Bullet("generate and print WordPress admin credentials")
			s.console.Bullet("finish WordPress installation via wp-cli")
		} else {
			s.console.Bullet("write starter index.php into " + docRoot)
		}
		if opts.WithLE {
			s.console.Bullet("perform domain reachability precheck for Let's Encrypt")
		}
		s.console.Success("Dry-run plan generated")
		return nil
	}

	if err := s.ensureRuntimeInstalled(phpVersion); err != nil {
		return err
	}

	if err := s.ensureDomainDoesNotExist(opts.Domain, vhostDir, serverConfigPath); err != nil {
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
		if err := ensureWordPressWithLSCache(docRoot); err != nil {
			return err
		}
		installedAccess, err := s.provisionWordPressInstall(ctx, opts.Domain, docRoot, phpVersion)
		if err != nil {
			return err
		}
		wpAccess = installedAccess
		s.console.Success("WordPress + LiteSpeed Cache provisioned")
	} else {
		if err := ensureStarterIndex(docRoot, opts.Domain); err != nil {
			return err
		}
	}

	if err := s.registerDomainInServerConfig(opts.Domain, siteRoot, vhostConfig, serverConfigPath); err != nil {
		return err
	}

	if opts.WithLE {
		if ok, detail := precheckLEDomainReachability(opts.Domain); ok {
			s.console.Success("Let's Encrypt precheck passed: domain is reachable over HTTP")
		} else {
			s.console.Warn("Let's Encrypt precheck failed: " + detail)
		}
		s.console.Warn("SSL issuance is not yet automated; run certbot and wire SSL listener manually")
	}

	s.console.Success("Virtual host files created")
	s.console.Bullet("Virtual host definition: " + vhostDefinition)
	s.console.Bullet("Virtual host config: " + vhostConfig)
	s.console.Bullet("Document root: " + docRoot)
	s.console.Bullet("Server config updated: " + serverConfigPath)
	if wpAccess != nil {
		s.console.Success("WordPress admin setup completed")
		s.console.Bullet("Admin URL: " + wpAccess.AdminURL)
		s.console.Bullet("Admin User: " + wpAccess.AdminUser)
		s.console.Bullet("Admin Password: " + wpAccess.AdminPassword)
		s.console.Bullet("Secrets file: " + wpAccess.SecretsFile)
	}
	s.console.Warn("Review listener mapping in OpenLiteSpeed WebAdmin and reload OLS")
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

	packages := packagesForPHPUpdate(phpVersion)
	vhostConfig := filepath.Join(s.lswsRoot, "conf", "vhosts", opts.Domain, "vhconf.conf")
	docRoot := filepath.Join(s.webRoot, opts.Domain, "html")

	s.console.Section("Update site PHP")
	s.console.Bullet("Domain: " + opts.Domain)
	s.console.Bullet("Target: lsphp" + phpVersion)
	s.console.Bullet("Platform: " + info.Summary())
	s.console.Bullet("VHost config: " + vhostConfig)
	if opts.WithWordPress {
		s.console.Bullet("WordPress + LiteSpeed Cache reconcile: enabled")
	}

	if opts.DryRun {
		s.console.Warn("Dry-run enabled: no system changes were made")
		s.console.Info("Planned package install:")
		for _, pkg := range packages {
			s.console.Bullet(pkg)
		}
		s.console.Info("Planned config operations:")
		s.console.Bullet("rewrite PHP handler in " + vhostConfig)
		if opts.WithWordPress {
			s.console.Bullet("ensure WordPress files exist in " + docRoot)
			s.console.Bullet("ensure LiteSpeed Cache plugin exists in " + filepath.Join(docRoot, "wp-content", "plugins", "litespeed-cache"))
		}
		s.console.Success("Dry-run plan generated")
		return nil
	}

	if !fileExists(vhostConfig) {
		return apperr.New(apperr.CodeValidation, fmt.Sprintf("virtual host does not exist for %s; expected %s", opts.Domain, vhostConfig))
	}

	if err := s.ensureRuntimeInstalled(phpVersion); err != nil {
		return err
	}

	installer := platform.NewPackageInstaller(s.runner, info)
	if err := installer.Install(ctx, packages...); err != nil {
		return err
	}

	if err := switchVHostPHPHandler(vhostConfig, phpVersion); err != nil {
		return err
	}

	if opts.WithWordPress {
		if err := ensureWordPressWithLSCache(docRoot); err != nil {
			return err
		}
		s.console.Success("WordPress + LiteSpeed Cache reconciled")
	}

	s.console.Success("Requested PHP package installed")
	s.console.Success("VHost PHP handler updated")
	s.console.Bullet("Reload OpenLiteSpeed to apply handler changes")
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

func (s SiteService) ensureDomainDoesNotExist(domain, vhostDir, serverConfigPath string) error {
	if fileExists(filepath.Join(vhostDir, "vhconf.conf")) || fileExists(filepath.Join(vhostDir, "vhost.conf")) {
		return apperr.New(apperr.CodeValidation, fmt.Sprintf("domain %s already exists in %s", domain, vhostDir))
	}

	b, err := os.ReadFile(serverConfigPath)
	if err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to read OpenLiteSpeed server config", err)
	}
	cfg := string(b)

	if strings.Contains(strings.ToLower(cfg), strings.ToLower("virtualhost "+domain+" {")) {
		return apperr.New(apperr.CodeValidation, fmt.Sprintf("domain %s already exists in %s", domain, serverConfigPath))
	}

	if hasDomainMapLine(cfg, domain) {
		return apperr.New(apperr.CodeValidation, fmt.Sprintf("domain %s is already mapped in %s", domain, serverConfigPath))
	}

	return nil
}

func (s SiteService) registerDomainInServerConfig(domain, siteRoot, vhostConfigPath, serverConfigPath string) error {
	b, err := os.ReadFile(serverConfigPath)
	if err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to read OpenLiteSpeed server config", err)
	}
	cfg := string(b)

	if strings.Contains(strings.ToLower(cfg), strings.ToLower("virtualhost "+domain+" {")) {
		return apperr.New(apperr.CodeValidation, fmt.Sprintf("domain %s already exists in %s", domain, serverConfigPath))
	}

	updated := strings.TrimRight(cfg, "\n") + "\n\n" + buildVHostDefinition(domain, siteRoot, vhostConfigPath) + "\n"
	updated, mappedHTTP := ensureDomainMappedInNamedListener(updated, "Default", domain)
	updated, mappedHTTPS := ensureDomainMappedInNamedListener(updated, "SSL", domain)

	mappedFallback := false
	if !mappedHTTP && !mappedHTTPS {
		updated, mappedFallback, err = ensureDomainMappedInFirstListener(updated, domain)
		if err != nil {
			return err
		}
	}

	if err := os.WriteFile(serverConfigPath, []byte(updated), 0o644); err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to update OpenLiteSpeed server config", err)
	}

	s.console.Bullet("Registered virtual host in " + serverConfigPath)
	if mappedHTTP {
		s.console.Bullet("Mapped domain in HTTP listener (Default): " + domain)
	}
	if mappedHTTPS {
		s.console.Bullet("Mapped domain in HTTPS listener (SSL): " + domain)
	}
	if mappedFallback {
		s.console.Bullet("Mapped domain in first listener: " + domain)
	}
	return nil
}

func ensureDomainMappedInNamedListener(cfg, listenerName, domain string) (string, bool) {
	lines := strings.Split(cfg, "\n")
	start, end := findListenerBlockByName(lines, listenerName)
	if start < 0 || end < 0 {
		return cfg, false
	}

	for i := start; i <= end; i++ {
		if mapLineContainsDomain(strings.TrimSpace(lines[i]), domain) {
			return cfg, false
		}
	}

	indent := detectMapIndent(lines[start : end+1])
	mapLine := fmt.Sprintf("%smap                     %s %s", indent, domain, domain)
	lines = append(lines[:end], append([]string{mapLine}, lines[end:]...)...)
	return strings.Join(lines, "\n"), true
}

func ensureDomainMappedInFirstListener(cfg, domain string) (string, bool, error) {
	lines := strings.Split(cfg, "\n")
	start, end := findFirstListenerBlock(lines)
	if start < 0 || end < 0 {
		return "", false, apperr.New(apperr.CodeConfig, "no listener block found in OpenLiteSpeed server config; cannot auto-map domain")
	}

	for i := start; i <= end; i++ {
		if mapLineContainsDomain(strings.TrimSpace(lines[i]), domain) {
			return cfg, false, nil
		}
	}

	indent := detectMapIndent(lines[start : end+1])
	mapLine := fmt.Sprintf("%smap                     %s %s", indent, domain, domain)
	lines = append(lines[:end], append([]string{mapLine}, lines[end:]...)...)
	return strings.Join(lines, "\n"), true, nil
}

func findFirstListenerBlock(lines []string) (int, int) {
	start := -1
	depth := 0

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if start == -1 {
			if strings.HasPrefix(trimmed, "listener ") && strings.Contains(trimmed, "{") {
				start = i
				depth = strings.Count(line, "{") - strings.Count(line, "}")
				if depth <= 0 {
					depth = 1
				}
			}
			continue
		}

		depth += strings.Count(line, "{")
		depth -= strings.Count(line, "}")
		if depth == 0 {
			return start, i
		}
	}

	return -1, -1
}

func detectMapIndent(listenerLines []string) string {
	for _, line := range listenerLines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "map ") {
			idx := strings.Index(line, "map")
			if idx > 0 {
				return line[:idx]
			}
		}
	}
	return "  "
}

func hasDomainMapLine(cfg, domain string) bool {
	for _, line := range strings.Split(cfg, "\n") {
		if mapLineContainsDomain(strings.TrimSpace(line), domain) {
			return true
		}
	}
	return false
}

func mapLineContainsDomain(line, domain string) bool {
	if !strings.HasPrefix(line, "map ") {
		return false
	}
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return false
	}
	for _, token := range fields[2:] {
		for _, host := range strings.Split(token, ",") {
			if strings.EqualFold(strings.TrimSpace(host), domain) {
				return true
			}
		}
	}
	return false
}

func switchVHostPHPHandler(vhostConfigPath, phpVersion string) error {
	b, err := os.ReadFile(vhostConfigPath)
	if err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to read vhost config", err)
	}

	content := string(b)
	newToken := "lsphp" + phpVersion
	if lsphpNamePattern.MatchString(content) {
		content = lsphpNamePattern.ReplaceAllString(content, newToken)
	} else {
		content = buildVHConfig(phpVersion)
	}

	if err := os.WriteFile(vhostConfigPath, []byte(content), 0o644); err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to write vhost config", err)
	}
	return nil
}

func precheckLEDomainReachability(domain string) (bool, string) {
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get("http://" + domain + "/")
	if err != nil {
		return false, err.Error()
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return false, fmt.Sprintf("http %d returned from domain", resp.StatusCode)
	}

	server := strings.ToLower(strings.TrimSpace(resp.Header.Get("Server")))
	if server != "" && !strings.Contains(server, "litespeed") && !strings.Contains(server, "cloudflare") {
		return false, fmt.Sprintf("server header %q does not look like LiteSpeed/cloudflare", server)
	}

	return true, fmt.Sprintf("http %d", resp.StatusCode)
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

func ensureWordPressWithLSCache(docRoot string) error {
	if !looksLikeWordPressDocRoot(docRoot) {
		if err := installWordPress(docRoot); err != nil {
			return err
		}
	}
	if err := installLiteSpeedCachePlugin(docRoot); err != nil {
		return err
	}
	return nil
}

func looksLikeWordPressDocRoot(docRoot string) bool {
	return fileExists(filepath.Join(docRoot, "wp-includes", "version.php")) && fileExists(filepath.Join(docRoot, "wp-admin", "admin.php"))
}

type wpAdminAccess struct {
	AdminURL      string
	AdminUser     string
	AdminPassword string
	SecretsFile   string
}

func (s SiteService) provisionWordPressInstall(ctx context.Context, domain, docRoot, phpVersion string) (*wpAdminAccess, error) {
	dbName, dbUser := deriveWordPressDBIdentifiers(domain)
	dbPassword, err := generateSecurePassword(24)
	if err != nil {
		return nil, err
	}
	adminPassword, err := generateSecurePassword(20)
	if err != nil {
		return nil, err
	}

	if err := s.createWordPressDatabase(ctx, dbName, dbUser, dbPassword); err != nil {
		return nil, err
	}

	phpPath, err := s.resolvePHPCLIPath(phpVersion)
	if err != nil {
		return nil, err
	}
	wpCLIPath := filepath.Join(docRoot, ".ols-wp-cli.phar")
	if err := ensureWPCLIPhar(wpCLIPath); err != nil {
		return nil, err
	}
	defer func() {
		_ = os.Remove(wpCLIPath)
	}()

	if !fileExists(filepath.Join(docRoot, "wp-config.php")) {
		if err := s.runWPCLI(ctx, phpPath, wpCLIPath,
			"core", "config",
			"--path="+docRoot,
			"--dbname="+dbName,
			"--dbuser="+dbUser,
			"--dbpass="+dbPassword,
			"--dbhost=localhost",
			"--skip-check",
			"--allow-root",
		); err != nil {
			return nil, err
		}
	}

	adminUser := "admin"
	adminEmail := "admin@" + domain
	if err := s.runWPCLI(ctx, phpPath, wpCLIPath,
		"core", "install",
		"--path="+docRoot,
		"--url=http://"+domain,
		"--title="+domain,
		"--admin_user="+adminUser,
		"--admin_password="+adminPassword,
		"--admin_email="+adminEmail,
		"--skip-email",
		"--allow-root",
	); err != nil {
		return nil, err
	}

	if err := s.runWPCLI(ctx, phpPath, wpCLIPath,
		"plugin", "activate", "litespeed-cache",
		"--path="+docRoot,
		"--allow-root",
	); err != nil {
		return nil, err
	}

	secretsPath, err := s.persistWordPressSecrets(domain, dbName, dbUser, dbPassword, adminUser, adminPassword)
	if err != nil {
		return nil, err
	}

	return &wpAdminAccess{
		AdminURL:      "http://" + domain + "/wp-admin",
		AdminUser:     adminUser,
		AdminPassword: adminPassword,
		SecretsFile:   secretsPath,
	}, nil
}

func (s SiteService) createWordPressDatabase(ctx context.Context, dbName, dbUser, dbPassword string) error {
	sql := fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS `%s`; CREATE USER IF NOT EXISTS '%s'@'localhost' IDENTIFIED BY '%s'; GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'localhost'; FLUSH PRIVILEGES;",
		dbName,
		escapeSQLString(dbUser),
		escapeSQLString(dbPassword),
		dbName,
		escapeSQLString(dbUser),
	)

	tmp, err := os.CreateTemp("", "ols-wp-sql-*.sql")
	if err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to prepare temporary SQL file", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.WriteString(sql + "\n"); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return apperr.Wrap(apperr.CodeConfig, "failed to write temporary SQL file", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return apperr.Wrap(apperr.CodeConfig, "failed to finalize temporary SQL file", err)
	}
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := s.runner.Run(ctx, "sh", "-c", "mysql < "+shellSingleQuote(tmpPath)); err == nil {
		return nil
	}
	if _, err := s.runner.Run(ctx, "sh", "-c", "mariadb < "+shellSingleQuote(tmpPath)); err == nil {
		return nil
	}

	return apperr.New(
		apperr.CodeCommand,
		"failed to create WordPress database/user; ensure local root DB access is available (sudo/root + mysql or mariadb client)",
	)
}

func (s SiteService) persistWordPressSecrets(domain, dbName, dbUser, dbPassword, adminUser, adminPassword string) (string, error) {
	secretsDir := filepath.Join(defaultSecretsRoot, domain)
	if err := os.MkdirAll(secretsDir, 0o700); err != nil {
		return "", apperr.Wrap(apperr.CodeConfig, "failed to create site secrets directory", err)
	}

	secretsPath := filepath.Join(secretsDir, "credentials.txt")
	content := fmt.Sprintf(
		"DOMAIN=%s\nDB_NAME=%s\nDB_USER=%s\nDB_PASSWORD=%s\nWP_ADMIN_USER=%s\nWP_ADMIN_PASSWORD=%s\n",
		domain,
		dbName,
		dbUser,
		dbPassword,
		adminUser,
		adminPassword,
	)
	if err := os.WriteFile(secretsPath, []byte(content), 0o600); err != nil {
		return "", apperr.Wrap(apperr.CodeConfig, "failed to persist site credentials", err)
	}
	return secretsPath, nil
}

func (s SiteService) resolvePHPCLIPath(phpVersion string) (string, error) {
	candidates := []string{
		filepath.Join(s.lswsRoot, "lsphp"+phpVersion, "bin", "php"),
		filepath.Join(s.lswsRoot, "lsphp"+phpVersion, "bin", "lsphp"),
	}
	for _, p := range candidates {
		if fileExists(p) {
			return p, nil
		}
	}
	return "", apperr.New(apperr.CodeValidation, fmt.Sprintf("no PHP CLI binary found for lsphp%s", phpVersion))
}

func ensureWPCLIPhar(wpCLIPath string) error {
	if fileExists(wpCLIPath) {
		return nil
	}

	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(wpCLIPharURL)
	if err != nil {
		return apperr.Wrap(apperr.CodeCommand, "failed to download wp-cli phar", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return apperr.New(apperr.CodeCommand, fmt.Sprintf("failed to download wp-cli phar: http %d", resp.StatusCode))
	}

	if err := os.MkdirAll(filepath.Dir(wpCLIPath), 0o755); err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to prepare wp-cli target path", err)
	}

	f, err := os.OpenFile(wpCLIPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to create wp-cli phar", err)
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		return apperr.Wrap(apperr.CodeConfig, "failed to write wp-cli phar", err)
	}
	if err := f.Close(); err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to finalize wp-cli phar", err)
	}
	return nil
}

func (s SiteService) runWPCLI(ctx context.Context, phpPath, wpCLIPath string, args ...string) error {
	cmdArgs := append([]string{wpCLIPath}, args...)
	res, err := s.runner.Run(ctx, phpPath, cmdArgs...)
	if err != nil {
		msg := strings.TrimSpace(strings.Join([]string{res.Stdout, res.Stderr}, "\n"))
		if msg != "" {
			return apperr.Wrap(apperr.CodeCommand, "wp-cli command failed: "+msg, err)
		}
		return apperr.Wrap(apperr.CodeCommand, "wp-cli command failed", err)
	}
	return nil
}

func deriveWordPressDBIdentifiers(domain string) (string, string) {
	base := strings.ToLower(strings.TrimSpace(domain))
	base = nonAlphaNumPattern.ReplaceAllString(base, "_")
	base = multiUnderscoreExpr.ReplaceAllString(base, "_")
	base = strings.Trim(base, "_")
	if base == "" {
		base = "site"
	}

	dbName := "wp_" + truncateString(base, 28)
	dbUser := "wpu_" + truncateString(base, 24)
	return dbName, dbUser
}

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func generateSecurePassword(length int) (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#%^*_-"
	if length <= 0 {
		return "", apperr.New(apperr.CodeValidation, "password length must be positive")
	}

	var b strings.Builder
	for i := 0; i < length; i++ {
		n, err := crand.Int(crand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", apperr.Wrap(apperr.CodeCommand, "failed to generate secure password", err)
		}
		b.WriteByte(alphabet[n.Int64()])
	}
	return b.String(), nil
}

func escapeSQLString(v string) string {
	return strings.ReplaceAll(v, "'", "''")
}

func shellSingleQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "'\\''") + "'"
}

func installLiteSpeedCachePlugin(docRoot string) error {
	const pluginURL = "https://downloads.wordpress.org/plugin/litespeed-cache.latest-stable.zip"

	pluginsRoot := filepath.Join(docRoot, "wp-content", "plugins")
	if err := os.MkdirAll(pluginsRoot, 0o755); err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to prepare plugin directory", err)
	}

	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(pluginURL)
	if err != nil {
		return apperr.Wrap(apperr.CodeCommand, "failed to download LiteSpeed Cache plugin", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return apperr.New(apperr.CodeCommand, fmt.Sprintf("failed to download LiteSpeed Cache plugin: http %d", resp.StatusCode))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return apperr.Wrap(apperr.CodeCommand, "failed to read LiteSpeed Cache plugin archive", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return apperr.Wrap(apperr.CodeCommand, "failed to open LiteSpeed Cache plugin archive", err)
	}

	root := filepath.Clean(pluginsRoot)
	for _, zf := range zr.File {
		cleanName := path.Clean(zf.Name)
		if cleanName == "." || cleanName == "litespeed-cache" {
			continue
		}
		if !strings.HasPrefix(cleanName, "litespeed-cache/") {
			continue
		}

		rel := strings.TrimPrefix(cleanName, "litespeed-cache/")
		if rel == "" {
			continue
		}

		target := filepath.Join(root, filepath.FromSlash(rel))
		cleanTarget := filepath.Clean(target)
		if cleanTarget != root && !strings.HasPrefix(cleanTarget, root+string(os.PathSeparator)) {
			return apperr.New(apperr.CodeConfig, "unsafe LiteSpeed Cache archive path detected")
		}

		if zf.FileInfo().IsDir() {
			if err := os.MkdirAll(cleanTarget, 0o755); err != nil {
				return apperr.Wrap(apperr.CodeConfig, "failed to create plugin directory", err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(cleanTarget), 0o755); err != nil {
			return apperr.Wrap(apperr.CodeConfig, "failed to create plugin file directory", err)
		}

		src, err := zf.Open()
		if err != nil {
			return apperr.Wrap(apperr.CodeConfig, "failed to open plugin file from archive", err)
		}

		mode := zf.Mode()
		if mode == 0 {
			mode = 0o644
		}
		dst, err := os.OpenFile(cleanTarget, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			_ = src.Close()
			return apperr.Wrap(apperr.CodeConfig, "failed to create plugin file", err)
		}

		if _, err := io.Copy(dst, src); err != nil {
			_ = dst.Close()
			_ = src.Close()
			return apperr.Wrap(apperr.CodeConfig, "failed to write plugin file", err)
		}

		if err := dst.Close(); err != nil {
			_ = src.Close()
			return apperr.Wrap(apperr.CodeConfig, "failed to finalize plugin file", err)
		}
		if err := src.Close(); err != nil {
			return apperr.Wrap(apperr.CodeConfig, "failed to close plugin archive stream", err)
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
		"81": {},
		"82": {},
		"83": {},
		"84": {},
		"85": {},
	}

	if _, ok := supported[candidate]; !ok {
		return "", apperr.New(
			apperr.CodeValidation,
			fmt.Sprintf("unsupported PHP version: %q (allowed: 81,82,83,84,85)", in),
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

func packagesForInstall(phpVersion, databasePackage string) []string {
	pkgs := []string{"openlitespeed", "lsphp" + phpVersion, "lsphp" + phpVersion + "-mysql"}
	if strings.TrimSpace(databasePackage) != "" {
		pkgs = append(pkgs, databasePackage)
	}
	return pkgs
}

func packagesForPHPUpdate(phpVersion string) []string {
	return []string{"lsphp" + phpVersion, "lsphp" + phpVersion + "-mysql"}
}
