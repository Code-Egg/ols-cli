package service

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	crand "crypto/rand"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"math/big"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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
	hexDigestPattern    = regexp.MustCompile(`(?i)[a-f0-9]{40,128}`)
	letsencryptLiveRoot = "/etc/letsencrypt/live"
)

const (
	defaultLSWSRoot              = "/usr/local/lsws"
	defaultWebRoot               = "/var/www"
	defaultSecretsRoot           = "/etc/ols-cli/sites"
	defaultOWASPCRSVersion       = "4.21.0"
	defaultOWASPCRSDirName       = "owasp-modsecurity-crs"
	defaultServerRecaptchaType   = 0
	defaultVHRecaptchaType       = 1
	defaultVHRecaptchaReqLimit   = 500
	defaultOWASPModSecRulesFile  = "/usr/local/lsws/conf/owasp/modsec_includes.conf"
	defaultModSecurityModuleFile = "/usr/local/lsws/modules/mod_security.so"
	defaultLiteSpeedRepoScript   = "https://repo.litespeed.sh"
	defaultRepoScriptTempPath    = "/tmp/ols-cli-litespeed-repo.sh"
	defaultHTTPUserAgent         = "ols-cli/0.1 (+https://github.com/ols/ols-cli)"
	defaultHTTPRetryAttempts     = 5
	defaultHTTPRetryMinDelay     = 2 * time.Second
	defaultHTTPRetryMaxDelay     = 30 * time.Second
	wpArchiveURL                 = "https://wordpress.org/latest.tar.gz"
	wpArchiveSHA1URL             = "https://wordpress.org/latest.tar.gz.sha1"
	wpCLIPharURL                 = "https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar"
	wpCLIPharSHA512URL           = "https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar.sha512"
)

type phpINISetting struct {
	key   string
	value string
}

var defaultPHPINISettings = []phpINISetting{
	{key: "post_max_size", value: "1000M"},
	{key: "upload_max_filesize", value: "1000M"},
	{key: "memory_limit", value: "1024M"},
	{key: "max_execution_time", value: "600"},
}

var defaultHSTSSecurityHeaders = []string{
	"Strict-Transport-Security: max-age=31536000; includeSubDomains",
	`Content-Security-Policy "upgrade-insecure-requests;connect-src *"`,
	"Referrer-Policy strict-origin-when-cross-origin",
	"X-Frame-Options: SAMEORIGIN",
	"X-Content-Type-Options: nosniff",
	"X-XSS-Protection 1;mode=block",
	`Permissions-Policy: geolocation=(self "")`,
}

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
	Domain            string
	WithWordPress     bool
	WithLE            bool
	PHPVersion        string
	OWASPEnabled      *bool
	RecaptchaEnabled  *bool
	NamespaceEnabled  *bool
	EnableHSTSHeaders bool
	DryRun            bool
}

type UpdateSiteOptions struct {
	Domain            string
	WithWordPress     bool
	PHPVersion        string
	OWASPEnabled      *bool
	RecaptchaEnabled  *bool
	NamespaceEnabled  *bool
	EnableHSTSHeaders bool
	DryRun            bool
}

type DeleteSiteOptions struct {
	Domain       string
	DropDatabase bool
	DryRun       bool
}

type SiteInfoOptions struct {
	Domain string
	DryRun bool
}

type ListSitesOptions struct {
	DryRun bool
}

type ShowSiteConfigOptions struct {
	Domain string
	DryRun bool
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
	s.console.Info("Preparing runtime installation plan")
	s.console.Bullet("Platform: " + info.Summary())
	s.console.Bullet("Config: " + plan.ConfigPath)
	s.console.Bullet("PHP: lsphp" + plan.PHPVersion)
	s.console.Bullet("Database: " + plan.DatabaseEngine)
	s.console.Bullet("OWASP CRS version: " + plan.OWASPCRSVersion)
	s.console.Bullet(fmt.Sprintf("VHost reCAPTCHA defaults: type=%d regConnLimit=%d", plan.VHRecaptchaType, plan.VHRecaptchaReqLimit))
	for _, p := range pkgs {
		s.console.Bullet("Package: " + p)
	}

	if opts.DryRun {
		s.console.Warn("Dry-run enabled: no system changes were made")
		s.console.Bullet("configure LiteSpeed package repository")
		s.console.Bullet("install runtime packages")
		s.console.Bullet("apply managed PHP ini defaults to all detected LiteSpeed runtimes")
		for _, setting := range defaultPHPINISettings {
			s.console.Bullet(fmt.Sprintf("php.ini: %s = %s", setting.key, setting.value))
		}
		if plan.ConfigureListeners {
			s.console.Bullet(fmt.Sprintf("configure listeners in %s/conf/httpd_config.conf", s.lswsRoot))
			s.console.Bullet(fmt.Sprintf("HTTP listener port: %d", plan.HTTPPort))
			s.console.Bullet(fmt.Sprintf("HTTPS listener port: %d", plan.HTTPSPort))
			s.console.Bullet("SSL cert: " + plan.SSLCertFile)
			s.console.Bullet("SSL key: " + plan.SSLKeyFile)
		} else {
			s.console.Bullet("skip listener configuration")
		}
		s.console.Bullet("install/ensure ModSecurity module package (ols-modsecurity)")
		s.console.Bullet("prepare OWASP CRS bundle and rules include at " + defaultOWASPModSecRulesFile)
		s.console.Bullet("configure server-level module mod_security with ls_enabled 0")
		s.console.Bullet(fmt.Sprintf("configure server-level lsrecaptcha with enabled=1 type=%d", defaultServerRecaptchaType))
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

	if err := s.applyDefaultPHPINISettings(); err != nil {
		return err
	}

	if plan.ConfigureListeners {
		if err := s.configureRuntimeListeners(plan.HTTPPort, plan.HTTPSPort, plan.SSLCertFile, plan.SSLKeyFile); err != nil {
			return err
		}
	}

	if err := s.ensureOWASPPrerequisites(ctx, info, plan.OWASPCRSVersion); err != nil {
		return err
	}

	serverConfigPath := filepath.Join(s.lswsRoot, "conf", "httpd_config.conf")
	if err := s.ensureServerSecurityDefaults(serverConfigPath); err != nil {
		return err
	}

	s.console.Success("OpenLiteSpeed runtime installed")
	s.console.Bullet("Binary: " + filepath.Join(s.lswsRoot, "bin", "lswsctrl"))
	s.console.Bullet("Default php.ini profile applied to detected LiteSpeed runtimes")
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
	logsRoot := filepath.Join(siteRoot, "logs")
	vhostDir := filepath.Join(s.lswsRoot, "conf", "vhosts", opts.Domain)
	vhostConfig := filepath.Join(vhostDir, "vhconf.conf")
	vhostDefinition := filepath.Join(vhostDir, "vhost.conf")
	serverConfigPath := filepath.Join(s.lswsRoot, "conf", "httpd_config.conf")
	siteURL := wordPressBaseURL(opts.Domain, opts.WithLE)
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
	if opts.OWASPEnabled != nil {
		s.console.Bullet("OWASP virtual-host mode: " + enabledLabel(*opts.OWASPEnabled))
	}
	if opts.RecaptchaEnabled != nil {
		s.console.Bullet("reCAPTCHA virtual-host mode: " + enabledLabel(*opts.RecaptchaEnabled))
	}
	if opts.NamespaceEnabled != nil {
		s.console.Bullet("Namespace virtual-host mode: " + enabledLabel(*opts.NamespaceEnabled))
	}
	if opts.EnableHSTSHeaders {
		s.console.Bullet("Security headers in context /: enabled")
	}

	if opts.DryRun {
		s.console.Warn("Dry-run enabled: no system changes were made")
		s.console.Info("Planned filesystem operations:")
		s.console.Bullet("check duplicate domain in " + serverConfigPath)
		s.console.Bullet("mkdir -p " + docRoot)
		s.console.Bullet("mkdir -p " + logsRoot)
		s.console.Bullet("mkdir -p " + vhostDir)
		s.console.Bullet("write " + vhostConfig)
		s.console.Bullet("write " + vhostDefinition)
		s.console.Bullet("append virtualhost block into " + serverConfigPath)
		s.console.Bullet("insert listener map in " + serverConfigPath)
		s.console.Bullet("align ownership to OpenLiteSpeed server user/group for " + siteRoot + " and " + vhostDir)
		if opts.OWASPEnabled != nil {
			s.console.Bullet("set virtual-host OWASP mod_security: " + enabledLabel(*opts.OWASPEnabled))
		}
		if opts.RecaptchaEnabled != nil {
			s.console.Bullet("set virtual-host LS reCAPTCHA: " + enabledLabel(*opts.RecaptchaEnabled))
		}
		if opts.NamespaceEnabled != nil {
			s.console.Bullet("set virtual-host namespace: " + enabledLabel(*opts.NamespaceEnabled))
		}
		if opts.EnableHSTSHeaders {
			s.console.Bullet("append recommended security extra headers to context / in " + vhostConfig)
		}
		if opts.WithWordPress {
			s.console.Bullet("download and extract WordPress into " + docRoot)
			s.console.Bullet("install and activate LiteSpeed Cache plugin via wp-cli")
			s.console.Bullet("create WordPress database and database user")
			s.console.Bullet("generate and print WordPress admin credentials")
			s.console.Bullet("finish WordPress installation via wp-cli")
			s.console.Bullet("force WordPress home/siteurl to " + siteURL)
		} else {
			s.console.Bullet("write starter index.php into " + docRoot)
		}
		if opts.WithLE {
			s.console.Bullet("perform domain reachability precheck for Let's Encrypt")
			s.console.Bullet("ensure certbot is installed")
			s.console.Bullet("issue Let's Encrypt certificate via certbot webroot challenge")
			s.console.Bullet("write cert/key into vhost SSL config")
		}
		s.console.Bullet("reload OpenLiteSpeed")
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
	if err := os.MkdirAll(logsRoot, 0o755); err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to create logs root", err)
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

	recaptchaType := defaultVHRecaptchaType
	recaptchaReqLimit := defaultVHRecaptchaReqLimit
	if boolPtrValue(opts.RecaptchaEnabled) {
		cfgType, cfgReqLimit, err := s.resolveVHostRecaptchaDefaultsFromConfig()
		if err != nil {
			s.console.Warn("Could not parse install config for vhost reCAPTCHA defaults; using built-in defaults")
		} else {
			recaptchaType = cfgType
			recaptchaReqLimit = cfgReqLimit
		}
	}

	changedSecurity, err := applyVHostSecurityOptions(vhostConfig, vhostSecurityOptions{
		OWASPEnabled:      opts.OWASPEnabled,
		RecaptchaEnabled:  opts.RecaptchaEnabled,
		NamespaceEnabled:  opts.NamespaceEnabled,
		RecaptchaType:     recaptchaType,
		RecaptchaReqLimit: recaptchaReqLimit,
		EnableHSTSHeaders: opts.EnableHSTSHeaders,
	})
	if err != nil {
		return err
	}
	if changedSecurity {
		s.console.Bullet("Applied requested security options in " + vhostConfig)
	}

	if opts.WithWordPress {
		if err := ensureWordPressFiles(docRoot); err != nil {
			return err
		}
		installedAccess, err := s.provisionWordPressInstall(ctx, opts.Domain, docRoot, phpVersion, siteURL)
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

	if err := s.applyServerConfiguredOwnership(serverConfigPath, siteRoot, vhostDir); err != nil {
		s.console.Warn("Could not align site ownership with OpenLiteSpeed user/group from server config: " + err.Error())
		if err := s.inheritOwnershipFromParent(siteRoot, vhostDir); err != nil {
			s.console.Warn("Could not align site ownership with parent directory ownership: " + err.Error())
		}
	}

	if err := s.registerDomainInServerConfig(opts.Domain, siteRoot, vhostConfig, serverConfigPath); err != nil {
		return err
	}
	if err := s.reloadOpenLiteSpeed(ctx); err != nil {
		s.console.Warn("Failed to reload OpenLiteSpeed automatically: " + err.Error())
	}

	if opts.WithLE {
		ok, detail := precheckLEDomainReachability(opts.Domain)
		if !ok {
			return apperr.New(apperr.CodeValidation, "Let's Encrypt precheck failed: "+detail)
		}
		s.console.Success("Let's Encrypt precheck passed: domain is reachable over HTTP")

		certFile, keyFile, err := s.issueLetsEncryptCertificate(ctx, info, opts.Domain, docRoot)
		if err != nil {
			return err
		}
		if err := applyVHostSSLCertificate(vhostConfig, certFile, keyFile); err != nil {
			return err
		}
		s.console.Success("Let's Encrypt certificate issued")
		s.console.Bullet("Certificate: " + certFile)
		s.console.Bullet("Private key: " + keyFile)

		if err := s.reloadOpenLiteSpeed(ctx); err != nil {
			s.console.Warn("Failed to reload OpenLiteSpeed automatically after SSL issuance: " + err.Error())
		}
	}

	if boolPtrValue(opts.RecaptchaEnabled) {
		recaptchaEnabled, err := isServerRecaptchaEnabled(serverConfigPath)
		if err != nil {
			s.console.Warn("Could not verify server-level reCAPTCHA switch: " + err.Error())
		} else if !recaptchaEnabled {
			s.console.Warn("Server-level reCAPTCHA is not enabled. Enable it in server config before vhost-level reCAPTCHA can take effect.")
		}
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
		s.console.Bullet("Admin Password: stored in secrets file")
		s.console.Bullet("Secrets file: " + wpAccess.SecretsFile)
	}
	return nil
}

func (s SiteService) UpdateSitePHP(ctx context.Context, opts UpdateSiteOptions) error {
	if err := ValidateDomain(opts.Domain); err != nil {
		return err
	}

	vhostConfig := filepath.Join(s.lswsRoot, "conf", "vhosts", opts.Domain, "vhconf.conf")
	docRoot := filepath.Join(s.webRoot, opts.Domain, "html")
	serverConfigPath := filepath.Join(s.lswsRoot, "conf", "httpd_config.conf")

	phpRequested := strings.TrimSpace(opts.PHPVersion) != ""
	securityRequested := opts.OWASPEnabled != nil || opts.RecaptchaEnabled != nil || opts.NamespaceEnabled != nil || opts.EnableHSTSHeaders

	if !phpRequested && !opts.WithWordPress && !securityRequested {
		return apperr.New(apperr.CodeValidation, "no update action requested")
	}
	if opts.WithWordPress && !phpRequested {
		return apperr.New(apperr.CodeValidation, "missing PHP version for --wp update flow")
	}

	var (
		phpVersion string
		packages   []string
		info       platform.Info
		err        error
	)
	if phpRequested {
		phpVersion, err = NormalizePHPVersion(opts.PHPVersion)
		if err != nil {
			return err
		}
		info, err = s.detector.Detect(ctx)
		if err != nil {
			return err
		}
		packages = packagesForPHPUpdate(phpVersion)
	}

	s.console.Section("Update site configuration")
	s.console.Bullet("Domain: " + opts.Domain)
	if phpRequested {
		s.console.Bullet("Target PHP: lsphp" + phpVersion)
		s.console.Bullet("Platform: " + info.Summary())
	}
	s.console.Bullet("VHost config: " + vhostConfig)
	if opts.WithWordPress {
		s.console.Bullet("WordPress + LiteSpeed Cache reconcile: enabled")
	}
	if opts.OWASPEnabled != nil {
		s.console.Bullet("OWASP virtual-host mode: " + enabledLabel(*opts.OWASPEnabled))
	}
	if opts.RecaptchaEnabled != nil {
		s.console.Bullet("reCAPTCHA virtual-host mode: " + enabledLabel(*opts.RecaptchaEnabled))
	}
	if opts.NamespaceEnabled != nil {
		s.console.Bullet("Namespace virtual-host mode: " + enabledLabel(*opts.NamespaceEnabled))
	}
	if opts.EnableHSTSHeaders {
		s.console.Bullet("Security headers in context /: enabled")
	}

	if opts.DryRun {
		s.console.Warn("Dry-run enabled: no system changes were made")
		if phpRequested {
			s.console.Info("Planned package install:")
			for _, pkg := range packages {
				s.console.Bullet(pkg)
			}
		}
		s.console.Info("Planned config operations:")
		if phpRequested {
			s.console.Bullet("rewrite PHP handler in " + vhostConfig)
		}
		if opts.WithWordPress {
			s.console.Bullet("ensure WordPress files exist in " + docRoot)
			s.console.Bullet("install and activate LiteSpeed Cache plugin via wp-cli")
		}
		if opts.OWASPEnabled != nil {
			s.console.Bullet("set virtual-host OWASP mod_security: " + enabledLabel(*opts.OWASPEnabled))
		}
		if opts.RecaptchaEnabled != nil {
			s.console.Bullet("set virtual-host LS reCAPTCHA: " + enabledLabel(*opts.RecaptchaEnabled))
		}
		if opts.NamespaceEnabled != nil {
			s.console.Bullet("set virtual-host namespace: " + enabledLabel(*opts.NamespaceEnabled))
		}
		if opts.EnableHSTSHeaders {
			s.console.Bullet("append recommended security extra headers to context / in " + vhostConfig)
		}
		s.console.Success("Dry-run plan generated")
		return nil
	}

	if !fileExists(vhostConfig) {
		return apperr.New(apperr.CodeValidation, fmt.Sprintf("virtual host does not exist for %s; expected %s", opts.Domain, vhostConfig))
	}

	if phpRequested {
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
	}

	if opts.WithWordPress {
		if err := ensureWordPressFiles(docRoot); err != nil {
			return err
		}
		if err := s.installAndActivateLiteSpeedCachePlugin(ctx, phpVersion, docRoot); err != nil {
			return err
		}
		s.console.Success("WordPress + LiteSpeed Cache reconciled")
	}

	recaptchaType := defaultVHRecaptchaType
	recaptchaReqLimit := defaultVHRecaptchaReqLimit
	if boolPtrValue(opts.RecaptchaEnabled) {
		cfgType, cfgReqLimit, err := s.resolveVHostRecaptchaDefaultsFromConfig()
		if err != nil {
			s.console.Warn("Could not parse install config for vhost reCAPTCHA defaults; using built-in defaults")
		} else {
			recaptchaType = cfgType
			recaptchaReqLimit = cfgReqLimit
		}
	}

	securityChanged, err := applyVHostSecurityOptions(vhostConfig, vhostSecurityOptions{
		OWASPEnabled:      opts.OWASPEnabled,
		RecaptchaEnabled:  opts.RecaptchaEnabled,
		NamespaceEnabled:  opts.NamespaceEnabled,
		RecaptchaType:     recaptchaType,
		RecaptchaReqLimit: recaptchaReqLimit,
		EnableHSTSHeaders: opts.EnableHSTSHeaders,
	})
	if err != nil {
		return err
	}

	if boolPtrValue(opts.RecaptchaEnabled) {
		recaptchaEnabled, err := isServerRecaptchaEnabled(serverConfigPath)
		if err != nil {
			s.console.Warn("Could not verify server-level reCAPTCHA switch: " + err.Error())
		} else if !recaptchaEnabled {
			s.console.Warn("Server-level reCAPTCHA is not enabled. Enable it in server config before vhost-level reCAPTCHA can take effect.")
		}
	}

	if phpRequested {
		s.console.Success("Requested PHP package installed")
		s.console.Success("VHost PHP handler updated")
	}
	if securityChanged {
		s.console.Success("Requested security options applied to vhost config")
	}
	s.console.Bullet("Reload OpenLiteSpeed to apply configuration changes")
	return nil
}

func (s SiteService) DeleteSite(ctx context.Context, opts DeleteSiteOptions) error {
	if err := ValidateDomain(opts.Domain); err != nil {
		return err
	}

	siteRoot := filepath.Join(s.webRoot, opts.Domain)
	vhostDir := filepath.Join(s.lswsRoot, "conf", "vhosts", opts.Domain)
	serverConfigPath := filepath.Join(s.lswsRoot, "conf", "httpd_config.conf")
	secretsPath := filepath.Join(defaultSecretsRoot, opts.Domain)

	s.console.Section("Delete site")
	s.console.Bullet("Domain: " + opts.Domain)
	s.console.Bullet("Document root: " + filepath.Join(siteRoot, "html"))
	s.console.Bullet("VHost directory: " + vhostDir)
	s.console.Bullet("Server config: " + serverConfigPath)
	if opts.DropDatabase {
		s.console.Bullet("Drop database: enabled")
	}

	if opts.DryRun {
		s.console.Warn("Dry-run enabled: no system changes were made")
		s.console.Bullet("remove domain maps and virtualhost block from " + serverConfigPath)
		s.console.Bullet("remove " + vhostDir)
		s.console.Bullet("remove " + siteRoot)
		s.console.Bullet("remove " + secretsPath)
		if opts.DropDatabase {
			s.console.Bullet("drop WordPress database and database user")
		}
		s.console.Bullet("reload OpenLiteSpeed")
		s.console.Success("Dry-run plan generated")
		return nil
	}

	if err := s.removeDomainFromServerConfig(opts.Domain, serverConfigPath); err != nil {
		return err
	}
	if err := removeIfExists(vhostDir); err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to remove virtual host directory", err)
	}
	if err := removeIfExists(siteRoot); err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to remove site root", err)
	}
	if err := removeIfExists(secretsPath); err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to remove site secrets", err)
	}

	if opts.DropDatabase {
		if err := s.dropWordPressDatabase(ctx, opts.Domain); err != nil {
			return err
		}
		s.console.Success("WordPress database/user removed")
	}

	if err := s.reloadOpenLiteSpeed(ctx); err != nil {
		s.console.Warn("Failed to reload OpenLiteSpeed automatically: " + err.Error())
	}

	s.console.Success("Site deleted")
	s.console.Bullet("Removed vhost directory: " + vhostDir)
	s.console.Bullet("Removed site root: " + siteRoot)
	s.console.Bullet("Server config updated: " + serverConfigPath)
	return nil
}

func (s SiteService) ShowSiteConfig(_ context.Context, opts ShowSiteConfigOptions) error {
	if err := ValidateDomain(opts.Domain); err != nil {
		return err
	}

	domain := strings.ToLower(strings.TrimSpace(opts.Domain))
	vhostConfig := filepath.Join(s.lswsRoot, "conf", "vhosts", domain, "vhconf.conf")
	if !fileExists(vhostConfig) {
		return apperr.New(apperr.CodeValidation, fmt.Sprintf("virtual host config not found for %s at %s", domain, vhostConfig))
	}

	if opts.DryRun {
		s.console.Warn("Dry-run flag has no side effects for read-only show output")
	}

	b, err := os.ReadFile(vhostConfig)
	if err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to read virtual host config", err)
	}

	s.console.Section("Site show")
	s.console.Bullet("Domain: " + domain)
	s.console.Bullet("VHost config: " + vhostConfig)
	for _, line := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		s.console.Bullet(line)
	}
	return nil
}

func (s SiteService) SiteInfo(_ context.Context, opts SiteInfoOptions) error {
	if err := ValidateDomain(opts.Domain); err != nil {
		return err
	}

	domain := strings.ToLower(strings.TrimSpace(opts.Domain))
	siteRoot := filepath.Join(s.webRoot, domain)
	docRoot := filepath.Join(siteRoot, "html")
	vhostDir := filepath.Join(s.lswsRoot, "conf", "vhosts", domain)
	vhostConfig := filepath.Join(vhostDir, "vhconf.conf")
	vhostDefinition := filepath.Join(vhostDir, "vhost.conf")
	serverConfigPath := filepath.Join(s.lswsRoot, "conf", "httpd_config.conf")
	secretsPath := filepath.Join(defaultSecretsRoot, domain, "credentials.txt")

	if !fileExists(vhostConfig) && !fileExists(vhostDefinition) && !fileExists(docRoot) {
		return apperr.New(apperr.CodeValidation, fmt.Sprintf("site %s not found", domain))
	}

	if opts.DryRun {
		s.console.Warn("Dry-run flag has no side effects for read-only info output")
	}

	phpVersion := "unknown"
	sslEnabled := false
	certFile := ""
	keyFile := ""
	if b, err := os.ReadFile(vhostConfig); err == nil {
		cfg := string(b)
		if token := lsphpNamePattern.FindString(cfg); token != "" {
			phpVersion = strings.TrimPrefix(token, "lsphp")
		}
		sslEnabled = strings.Contains(cfg, "vhssl")
		certFile = firstDirectiveValue(cfg, "certFile")
		keyFile = firstDirectiveValue(cfg, "keyFile")
	}

	serverMapped := false
	if b, err := os.ReadFile(serverConfigPath); err == nil {
		cfg := string(b)
		serverMapped = strings.Contains(strings.ToLower(cfg), strings.ToLower("virtualhost "+domain+" {")) || hasDomainMapLine(cfg, domain)
	}

	wpDetected := looksLikeWordPressDocRoot(docRoot)
	secrets := readSecretsFile(secretsPath)
	dbName := secrets["DB_NAME"]
	dbUser := secrets["DB_USER"]

	s.console.Section("Site info")
	s.console.Bullet("Domain: " + domain)
	s.console.Bullet("Document root: " + docRoot)
	s.console.Bullet("VHost config: " + vhostConfig)
	s.console.Bullet("VHost definition: " + vhostDefinition)
	s.console.Bullet("Server mapping: " + yesNo(serverMapped))
	if phpVersion == "unknown" {
		s.console.Bullet("PHP version: unknown")
	} else {
		s.console.Bullet("PHP version: lsphp" + phpVersion)
	}
	s.console.Bullet("WordPress detected: " + yesNo(wpDetected))
	s.console.Bullet("SSL enabled: " + yesNo(sslEnabled))
	if certFile != "" {
		s.console.Bullet("SSL certFile: " + certFile)
	}
	if keyFile != "" {
		s.console.Bullet("SSL keyFile: " + keyFile)
	}
	if dbName != "" {
		s.console.Bullet("Database name: " + dbName)
	}
	if dbUser != "" {
		s.console.Bullet("Database user: " + dbUser)
	}

	return nil
}

func (s SiteService) ListSites(_ context.Context, opts ListSitesOptions) error {
	vhostRoot := filepath.Join(s.lswsRoot, "conf", "vhosts")
	entries, err := os.ReadDir(vhostRoot)
	if err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to read OpenLiteSpeed vhost directory", err)
	}

	if opts.DryRun {
		s.console.Warn("Dry-run flag has no side effects for read-only list output")
	}

	domains := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		domain := strings.TrimSpace(entry.Name())
		if domain == "" {
			continue
		}
		if !fileExists(filepath.Join(vhostRoot, domain, "vhconf.conf")) {
			continue
		}
		domains = append(domains, domain)
	}

	sort.Strings(domains)

	s.console.Section("Site list")
	s.console.Bullet("VHost root: " + vhostRoot)
	if len(domains) == 0 {
		s.console.Warn("No managed sites found")
		return nil
	}

	for _, domain := range domains {
		docRoot := filepath.Join(s.webRoot, domain, "html")
		wpTag := ""
		if looksLikeWordPressDocRoot(docRoot) {
			wpTag = " [wp]"
		}
		s.console.Bullet(domain + wpTag)
	}

	s.console.Success(fmt.Sprintf("Found %d site(s)", len(domains)))
	return nil
}

func readSecretsFile(path string) map[string]string {
	result := map[string]string{}
	b, err := os.ReadFile(path)
	if err != nil {
		return result
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if key == "" {
			continue
		}
		result[key] = val
	}
	return result
}

func yesNo(ok bool) string {
	if ok {
		return "yes"
	}
	return "no"
}

func (s SiteService) applyDefaultPHPINISettings() error {
	iniPaths, err := discoverLiteSpeedPHPINIPaths(s.lswsRoot)
	if err != nil {
		return err
	}
	if len(iniPaths) == 0 {
		s.console.Warn("No LiteSpeed php.ini files were found under " + s.lswsRoot + "; skipped applying defaults")
		return nil
	}

	for _, iniPath := range iniPaths {
		if err := applyPHPINISettingsFile(iniPath, defaultPHPINISettings); err != nil {
			return err
		}
		s.console.Bullet("Applied php.ini defaults: " + iniPath)
	}
	return nil
}

func discoverLiteSpeedPHPINIPaths(lswsRoot string) ([]string, error) {
	matches := map[string]struct{}{}

	phpRoots, err := filepath.Glob(filepath.Join(lswsRoot, "lsphp*", "etc", "php"))
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeConfig, "failed to discover LiteSpeed php.ini roots", err)
	}
	for _, root := range phpRoots {
		walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			if strings.EqualFold(d.Name(), "php.ini") {
				matches[path] = struct{}{}
			}
			return nil
		})
		if walkErr != nil {
			return nil, apperr.Wrap(apperr.CodeConfig, "failed while scanning LiteSpeed php.ini files", walkErr)
		}
	}

	fallbackPattern := filepath.Join(lswsRoot, "lsphp*", "etc", "php.ini")
	fallbackPaths, err := filepath.Glob(fallbackPattern)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeConfig, "failed to discover fallback LiteSpeed php.ini files", err)
	}
	for _, p := range fallbackPaths {
		matches[p] = struct{}{}
	}

	paths := make([]string, 0, len(matches))
	for p := range matches {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths, nil
}

func applyPHPINISettingsFile(path string, settings []phpINISetting) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to read php.ini", err)
	}
	content := string(b)
	changed := false
	for _, setting := range settings {
		updated, didChange := upsertINIValue(content, setting.key, setting.value)
		if didChange {
			changed = true
			content = updated
		}
	}
	if !changed {
		return nil
	}

	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to write php.ini", err)
	}
	return nil
}

func upsertINIValue(content, key, value string) (string, bool) {
	line := fmt.Sprintf("%s = %s", key, value)
	pattern := regexp.MustCompile(`(?im)^\s*;?\s*` + regexp.QuoteMeta(key) + `\s*=.*$`)
	if pattern.MatchString(content) {
		updated := pattern.ReplaceAllString(content, line)
		return updated, updated != content
	}

	trimmed := strings.TrimRight(content, "\n")
	if trimmed == "" {
		return line + "\n", true
	}
	return trimmed + "\n" + line + "\n", true
}

func (s SiteService) configureLiteSpeedRepo(ctx context.Context, info platform.Info) error {
	if err := verifyLiteSpeedPackageAvailability(ctx, s.runner, info); err == nil {
		s.console.Bullet("LiteSpeed package repository is already configured")
		return nil
	}
	s.console.Bullet("Configuring LiteSpeed package repository")
	if err := s.bootstrapLiteSpeedRepo(ctx); err != nil {
		return err
	}
	if err := verifyLiteSpeedPackageAvailability(ctx, s.runner, info); err != nil {
		return apperr.Wrap(
			apperr.CodeCommand,
			"LiteSpeed repository bootstrap completed but openlitespeed is still unavailable",
			err,
		)
	}
	s.console.Bullet("LiteSpeed package repository configured")
	return nil
}

func (s SiteService) bootstrapLiteSpeedRepo(ctx context.Context) error {
	if err := downloadToFile(defaultLiteSpeedRepoScript, defaultRepoScriptTempPath, 0o700); err != nil {
		return apperr.Wrap(apperr.CodeCommand, "failed to download LiteSpeed repository bootstrap script", err)
	}
	defer func() {
		_ = os.Remove(defaultRepoScriptTempPath)
	}()

	if _, err := s.runner.Run(ctx, "bash", defaultRepoScriptTempPath); err != nil {
		return apperr.Wrap(apperr.CodeCommand, "failed to execute LiteSpeed repository bootstrap script", err)
	}
	return nil
}

func verifyLiteSpeedPackageAvailability(ctx context.Context, run runner.Runner, info platform.Info) error {
	switch info.PackageManager {
	case platform.PackageManagerAPT:
		if _, err := run.Run(ctx, "apt-cache", "show", "openlitespeed"); err != nil {
			return apperr.Wrap(
				apperr.CodeCommand,
				"openlitespeed package is not available via apt repositories",
				err,
			)
		}
	case platform.PackageManagerYUM:
		if _, err := run.Run(ctx, "yum", "-q", "info", "openlitespeed"); err != nil {
			return apperr.Wrap(
				apperr.CodeCommand,
				"openlitespeed package is not available via yum repositories",
				err,
			)
		}
	case platform.PackageManagerDNF:
		if _, err := run.Run(ctx, "dnf", "-q", "info", "openlitespeed"); err != nil {
			return apperr.Wrap(
				apperr.CodeCommand,
				"openlitespeed package is not available via dnf repositories",
				err,
			)
		}
	default:
		return apperr.New(apperr.CodePlatform, fmt.Sprintf("unsupported package manager: %s", info.PackageManager))
	}
	return nil
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
	lines := strings.Split(updated, "\n")
	httpListenerName := chooseExistingListenerName(lines, []string{"HTTP", "Default"}, "HTTP")
	httpsListenerName := chooseExistingListenerName(lines, []string{"HTTPS", "SSL"}, "HTTPS")

	updated, mappedHTTP := ensureDomainMappedInNamedListener(updated, httpListenerName, domain)
	updated, mappedHTTPS := ensureDomainMappedInNamedListener(updated, httpsListenerName, domain)

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
		s.console.Bullet("Mapped domain in HTTP listener (" + httpListenerName + "): " + domain)
	}
	if mappedHTTPS {
		s.console.Bullet("Mapped domain in HTTPS listener (" + httpsListenerName + "): " + domain)
	}
	if mappedFallback {
		s.console.Bullet("Mapped domain in first listener: " + domain)
	}
	return nil
}

func (s SiteService) removeDomainFromServerConfig(domain, serverConfigPath string) error {
	b, err := os.ReadFile(serverConfigPath)
	if err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to read OpenLiteSpeed server config", err)
	}
	cfg := string(b)

	updated, removedVHost, err := removeVirtualHostBlock(cfg, domain)
	if err != nil {
		return err
	}
	updated, removedMaps := removeDomainMappings(updated, domain)
	if !removedVHost && !removedMaps {
		s.console.Warn("No matching virtualhost/map entries found for " + domain)
		return nil
	}

	if err := os.WriteFile(serverConfigPath, []byte(updated), 0o644); err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to update OpenLiteSpeed server config", err)
	}

	s.console.Bullet("Removed domain config from " + serverConfigPath)
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

func removeVirtualHostBlock(cfg, domain string) (string, bool, error) {
	lines := strings.Split(cfg, "\n")
	start, end := findVirtualHostBlockByName(lines, domain)
	if start < 0 || end < 0 {
		return cfg, false, nil
	}
	updatedLines := append([]string{}, lines[:start]...)
	updatedLines = append(updatedLines, lines[end+1:]...)
	return strings.Join(updatedLines, "\n"), true, nil
}

func findVirtualHostBlockByName(lines []string, domain string) (int, int) {
	start := -1
	depth := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if start == -1 {
			if !strings.HasPrefix(trimmed, "virtualhost ") || !strings.Contains(trimmed, "{") {
				continue
			}
			header := strings.TrimSpace(strings.SplitN(trimmed, "{", 2)[0])
			parts := strings.Fields(header)
			if len(parts) < 2 || !strings.EqualFold(parts[1], domain) {
				continue
			}
			start = i
			depth = strings.Count(line, "{") - strings.Count(line, "}")
			if depth <= 0 {
				depth = 1
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

func removeDomainMappings(cfg, domain string) (string, bool) {
	lines := strings.Split(cfg, "\n")
	changed := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "map ") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 3 {
			continue
		}
		mapName := fields[1]
		keptHosts := make([]string, 0, len(fields)-2)
		removed := false
		for _, token := range fields[2:] {
			for _, host := range strings.Split(token, ",") {
				host = strings.TrimSpace(host)
				if host == "" {
					continue
				}
				if strings.EqualFold(host, domain) {
					removed = true
					continue
				}
				keptHosts = append(keptHosts, host)
			}
		}
		if !removed {
			continue
		}
		changed = true
		if len(keptHosts) == 0 {
			lines[i] = ""
			continue
		}
		indent := ""
		if idx := strings.Index(line, "map"); idx > 0 {
			indent = line[:idx]
		}
		lines[i] = fmt.Sprintf("%smap                     %s %s", indent, mapName, strings.Join(keptHosts, ","))
	}
	if !changed {
		return cfg, false
	}
	return strings.Join(lines, "\n"), true
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

type vhostSecurityOptions struct {
	OWASPEnabled      *bool
	RecaptchaEnabled  *bool
	NamespaceEnabled  *bool
	RecaptchaType     int
	RecaptchaReqLimit int
	EnableHSTSHeaders bool
}

func boolPtrValue(v *bool) bool {
	return v != nil && *v
}

func enabledLabel(v bool) string {
	if v {
		return "enabled"
	}
	return "disabled"
}

func owaspCRSArchiveURL(version string) string {
	return fmt.Sprintf("https://github.com/coreruleset/coreruleset/archive/refs/tags/v%s.zip", strings.TrimSpace(version))
}

func (s SiteService) ensureOWASPPrerequisites(ctx context.Context, info platform.Info, owaspCRSVersion string) error {
	owaspCRSVersion = strings.TrimSpace(owaspCRSVersion)
	if owaspCRSVersion == "" {
		owaspCRSVersion = defaultOWASPCRSVersion
	}
	if !fileExists(defaultModSecurityModuleFile) {
		s.console.Bullet("Installing ModSecurity module package: ols-modsecurity")
		installer := platform.NewPackageInstaller(s.runner, info)
		if err := installer.Install(ctx, "ols-modsecurity"); err != nil {
			return apperr.Wrap(apperr.CodeCommand, "failed to install ols-modsecurity package", err)
		}
	}

	if !fileExists(defaultModSecurityModuleFile) {
		return apperr.New(apperr.CodeValidation, fmt.Sprintf("ModSecurity module not found at %s after installation attempt", defaultModSecurityModuleFile))
	}

	changed, err := ensureOWASPRulesBundle(defaultOWASPModSecRulesFile, owaspCRSVersion)
	if err != nil {
		return err
	}
	if changed {
		s.console.Bullet("Prepared OWASP CRS rules bundle: " + defaultOWASPModSecRulesFile)
	}
	return nil
}

func (s SiteService) resolveVHostRecaptchaDefaultsFromConfig() (int, int, error) {
	cfg, _, err := loadRuntimeInstallConfig("", s.lswsRoot)
	if err != nil {
		return 0, 0, err
	}
	recaptchaType := cfg.VHRecaptchaType
	if recaptchaType <= 0 {
		recaptchaType = defaultVHRecaptchaType
	}
	recaptchaReqLimit := cfg.VHRecaptchaReqLimit
	if recaptchaReqLimit <= 0 {
		recaptchaReqLimit = defaultVHRecaptchaReqLimit
	}
	return recaptchaType, recaptchaReqLimit, nil
}

func ensureOWASPRulesBundle(rulesPath, crsVersion string) (bool, error) {
	owaspDir := filepath.Dir(rulesPath)
	crsDir := filepath.Join(owaspDir, defaultOWASPCRSDirName)
	changed := false
	versionMarker := filepath.Join(owaspDir, ".ols-crs-version")

	if err := os.MkdirAll(owaspDir, 0o755); err != nil {
		return false, apperr.Wrap(apperr.CodeConfig, "failed to create OWASP directory", err)
	}

	needInstall := !fileExists(filepath.Join(crsDir, "rules"))
	if !needInstall {
		if markerBytes, err := os.ReadFile(versionMarker); err == nil {
			if strings.TrimSpace(string(markerBytes)) != strings.TrimSpace(crsVersion) {
				needInstall = true
			}
		} else if !os.IsNotExist(err) {
			return false, apperr.Wrap(apperr.CodeConfig, "failed to read OWASP CRS version marker", err)
		}
	}

	if needInstall {
		if err := installOWASPCRSBundle(owaspDir, crsDir, crsVersion); err != nil {
			return false, err
		}
		changed = true
	}

	if err := promoteOWASPExampleFiles(crsDir); err != nil {
		return false, err
	}

	rulesDir := filepath.Join(crsDir, "rules")
	entries, err := os.ReadDir(rulesDir)
	if err != nil {
		return false, apperr.Wrap(apperr.CodeConfig, "failed to read OWASP rules directory", err)
	}

	ruleFiles := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if strings.HasPrefix(name, "REQUEST-") || strings.HasPrefix(name, "RESPONSE-") {
			ruleFiles = append(ruleFiles, name)
		}
	}
	if len(ruleFiles) == 0 {
		return false, apperr.New(apperr.CodeConfig, fmt.Sprintf("no OWASP CRS REQUEST/RESPONSE rules found in %s", rulesDir))
	}
	sort.Strings(ruleFiles)

	modsecPath := filepath.Join(owaspDir, "modsecurity.conf")
	modsecChanged, err := writeFileIfChanged(modsecPath, []byte("SecRuleEngine On\n"), 0o644)
	if err != nil {
		return false, err
	}
	changed = changed || modsecChanged

	var includes strings.Builder
	includes.WriteString("include modsecurity.conf\n")
	includes.WriteString("include " + defaultOWASPCRSDirName + "/crs-setup.conf\n")
	for _, file := range ruleFiles {
		includes.WriteString("include " + defaultOWASPCRSDirName + "/rules/" + file + "\n")
	}
	includesChanged, err := writeFileIfChanged(rulesPath, []byte(includes.String()), 0o644)
	if err != nil {
		return false, err
	}
	changed = changed || includesChanged

	versionChanged, err := writeFileIfChanged(versionMarker, []byte(strings.TrimSpace(crsVersion)+"\n"), 0o644)
	if err != nil {
		return false, err
	}
	changed = changed || versionChanged

	return changed, nil
}

func installOWASPCRSBundle(owaspDir, targetDir, crsVersion string) error {
	tmp, err := os.CreateTemp("", "ols-owasp-crs-*.zip")
	if err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to create temporary OWASP archive path", err)
	}
	archivePath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(archivePath)
		return apperr.Wrap(apperr.CodeConfig, "failed to initialize temporary OWASP archive", err)
	}
	defer func() {
		_ = os.Remove(archivePath)
	}()

	if err := downloadToFile(owaspCRSArchiveURL(crsVersion), archivePath, 0o644); err != nil {
		return apperr.Wrap(apperr.CodeCommand, "failed to download OWASP CRS archive", err)
	}

	extractedRoot, err := extractZipArchive(archivePath, owaspDir)
	if err != nil {
		return err
	}
	if extractedRoot == "" {
		return apperr.New(apperr.CodeConfig, "OWASP CRS archive extraction produced no root directory")
	}

	sourceDir := filepath.Join(owaspDir, extractedRoot)
	if !fileExists(sourceDir) {
		return apperr.New(apperr.CodeConfig, fmt.Sprintf("expected extracted OWASP directory %s was not found", sourceDir))
	}

	if fileExists(targetDir) {
		if err := os.RemoveAll(targetDir); err != nil {
			return apperr.Wrap(apperr.CodeConfig, "failed to remove existing OWASP CRS directory", err)
		}
	}

	if strings.EqualFold(filepath.Clean(sourceDir), filepath.Clean(targetDir)) {
		return nil
	}

	if err := os.Rename(sourceDir, targetDir); err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to place OWASP CRS directory", err)
	}
	return nil
}

func extractZipArchive(zipPath, destDir string) (string, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", apperr.Wrap(apperr.CodeConfig, "failed to open OWASP CRS archive", err)
	}
	defer reader.Close()

	cleanDest := filepath.Clean(destDir)
	topRoot := ""

	for _, file := range reader.File {
		archiveName := strings.ReplaceAll(strings.TrimSpace(file.Name), "\\", "/")
		if archiveName == "" {
			continue
		}

		rel := path.Clean(strings.TrimPrefix(archiveName, "/"))
		if rel == "." || rel == "" {
			continue
		}
		if strings.HasPrefix(rel, "../") || rel == ".." {
			return "", apperr.New(apperr.CodeConfig, "unsafe OWASP archive path detected")
		}

		parts := strings.Split(rel, "/")
		if topRoot == "" && len(parts) > 0 {
			topRoot = parts[0]
		}

		target := filepath.Join(cleanDest, filepath.FromSlash(rel))
		cleanTarget := filepath.Clean(target)
		if cleanTarget != cleanDest && !strings.HasPrefix(cleanTarget, cleanDest+string(os.PathSeparator)) {
			return "", apperr.New(apperr.CodeConfig, "unsafe OWASP archive extraction target detected")
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(cleanTarget, 0o755); err != nil {
				return "", apperr.Wrap(apperr.CodeConfig, "failed to create OWASP extracted directory", err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(cleanTarget), 0o755); err != nil {
			return "", apperr.Wrap(apperr.CodeConfig, "failed to create OWASP extracted file directory", err)
		}

		mode := file.Mode()
		if mode == 0 {
			mode = 0o644
		}

		in, err := file.Open()
		if err != nil {
			return "", apperr.Wrap(apperr.CodeConfig, "failed to open OWASP archive entry", err)
		}
		out, err := os.OpenFile(cleanTarget, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			_ = in.Close()
			return "", apperr.Wrap(apperr.CodeConfig, "failed to create OWASP extracted file", err)
		}
		if _, err := io.Copy(out, in); err != nil {
			_ = in.Close()
			_ = out.Close()
			return "", apperr.Wrap(apperr.CodeConfig, "failed to write OWASP extracted file", err)
		}
		if err := in.Close(); err != nil {
			_ = out.Close()
			return "", apperr.Wrap(apperr.CodeConfig, "failed to close OWASP archive entry", err)
		}
		if err := out.Close(); err != nil {
			return "", apperr.Wrap(apperr.CodeConfig, "failed to finalize OWASP extracted file", err)
		}
	}

	return topRoot, nil
}

func promoteOWASPExampleFiles(crsDir string) error {
	replacements := []struct {
		example string
		target  string
	}{
		{
			example: filepath.Join(crsDir, "rules", "REQUEST-900-EXCLUSION-RULES-BEFORE-CRS.conf.example"),
			target:  filepath.Join(crsDir, "rules", "REQUEST-900-EXCLUSION-RULES-BEFORE-CRS.conf"),
		},
		{
			example: filepath.Join(crsDir, "rules", "RESPONSE-999-EXCLUSION-RULES-AFTER-CRS.conf.example"),
			target:  filepath.Join(crsDir, "rules", "RESPONSE-999-EXCLUSION-RULES-AFTER-CRS.conf"),
		},
		{
			example: filepath.Join(crsDir, "crs-setup.conf.example"),
			target:  filepath.Join(crsDir, "crs-setup.conf"),
		},
	}

	for _, item := range replacements {
		if fileExists(item.target) || !fileExists(item.example) {
			continue
		}
		if err := os.Rename(item.example, item.target); err != nil {
			return apperr.Wrap(apperr.CodeConfig, "failed to promote OWASP example file", err)
		}
	}
	return nil
}

func writeFileIfChanged(path string, content []byte, mode os.FileMode) (bool, error) {
	existing, err := os.ReadFile(path)
	if err == nil {
		if string(existing) == string(content) {
			return false, nil
		}
	} else if !os.IsNotExist(err) {
		return false, apperr.Wrap(apperr.CodeConfig, "failed to read file for update check", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, apperr.Wrap(apperr.CodeConfig, "failed to create directory for file write", err)
	}
	if err := os.WriteFile(path, content, mode); err != nil {
		return false, apperr.Wrap(apperr.CodeConfig, "failed to write file", err)
	}
	return true, nil
}

func isTruthyDirectiveValue(v string) bool {
	switch strings.TrimSpace(strings.ToLower(v)) {
	case "1", "on", "yes", "true":
		return true
	default:
		return false
	}
}

func isServerRecaptchaEnabled(serverConfigPath string) (bool, error) {
	b, err := os.ReadFile(serverConfigPath)
	if err != nil {
		return false, apperr.Wrap(apperr.CodeConfig, "failed to read OpenLiteSpeed server config", err)
	}
	lines := strings.Split(string(b), "\n")
	start, end := findBlockByKey(lines, "lsrecaptcha")
	if start < 0 || end < 0 {
		return false, nil
	}
	for _, line := range lines[start+1 : end] {
		trimmed := strings.TrimSpace(line)
		fields := strings.Fields(trimmed)
		if len(fields) < 2 || !strings.EqualFold(fields[0], "enabled") {
			continue
		}
		return isTruthyDirectiveValue(fields[1]), nil
	}
	return false, nil
}

func buildServerModSecurityDefaultBlock() []string {
	return []string{
		"module mod_security {",
		formatDirectiveLine("  ", "modsecurity", "on"),
		"  modsecurity_rules       `",
		"  SecRuleEngine On",
		"  `",
		formatDirectiveLine("  ", "modsecurity_rules_file", defaultOWASPModSecRulesFile),
		formatDirectiveLine("  ", "ls_enabled", "0"),
		"}",
	}
}

func buildServerRecaptchaDefaultBlock() []string {
	return []string{
		"lsrecaptcha  {",
		formatDirectiveLine("  ", "enabled", "1"),
		formatDirectiveLine("  ", "type", fmt.Sprintf("%d", defaultServerRecaptchaType)),
		"}",
	}
}

func upsertServerDefaultBlock(lines []string, key string, block []string) ([]string, bool) {
	start, end := findBlockByKey(lines, key)
	if start < 0 || end < 0 {
		return appendBlock(lines, block), true
	}
	current := append([]string{}, lines[start:end+1]...)
	if sameStringSlice(current, block) {
		return lines, false
	}
	return replaceBlock(lines, start, end, block), true
}

func (s SiteService) ensureServerSecurityDefaults(serverConfigPath string) error {
	b, err := os.ReadFile(serverConfigPath)
	if err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to read OpenLiteSpeed server config", err)
	}

	lines := strings.Split(string(b), "\n")
	changed := false

	lines, modsecChanged := upsertServerDefaultBlock(lines, "module mod_security", buildServerModSecurityDefaultBlock())
	changed = changed || modsecChanged
	lines, recaptchaChanged := upsertServerDefaultBlock(lines, "lsrecaptcha", buildServerRecaptchaDefaultBlock())
	changed = changed || recaptchaChanged
	lines, namespaceChanged := upsertDirective(lines, "namespace", "1")
	changed = changed || namespaceChanged

	if !changed {
		return nil
	}
	if err := os.WriteFile(serverConfigPath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to update OpenLiteSpeed server security defaults", err)
	}
	s.console.Bullet("Configured server-level ModSecurity (ls_enabled=0), reCAPTCHA defaults, and namespace=1 in " + serverConfigPath)
	return nil
}

func applyVHostSecurityOptions(vhostConfigPath string, opts vhostSecurityOptions) (bool, error) {
	if opts.OWASPEnabled == nil && opts.RecaptchaEnabled == nil && opts.NamespaceEnabled == nil && !opts.EnableHSTSHeaders {
		return false, nil
	}

	b, err := os.ReadFile(vhostConfigPath)
	if err != nil {
		return false, apperr.Wrap(apperr.CodeConfig, "failed to read vhost config for security updates", err)
	}

	lines := strings.Split(string(b), "\n")
	changed := false

	if opts.OWASPEnabled != nil {
		updated, blockChanged, err := upsertVHostOWASPBlock(lines, *opts.OWASPEnabled)
		if err != nil {
			return false, err
		}
		lines = updated
		changed = changed || blockChanged
	}

	if opts.RecaptchaEnabled != nil {
		recaptchaType := opts.RecaptchaType
		if recaptchaType <= 0 {
			recaptchaType = defaultVHRecaptchaType
		}
		recaptchaReqLimit := opts.RecaptchaReqLimit
		if recaptchaReqLimit <= 0 {
			recaptchaReqLimit = defaultVHRecaptchaReqLimit
		}
		updated, blockChanged, err := upsertVHostRecaptchaBlock(lines, *opts.RecaptchaEnabled, recaptchaType, recaptchaReqLimit)
		if err != nil {
			return false, err
		}
		lines = updated
		changed = changed || blockChanged
	}

	if opts.NamespaceEnabled != nil {
		updated, namespaceChanged := upsertVHostNamespaceDirective(lines, *opts.NamespaceEnabled)
		lines = updated
		changed = changed || namespaceChanged
	}

	if opts.EnableHSTSHeaders {
		updated, headersChanged, err := ensureRootContextExtraHeaders(lines, defaultHSTSSecurityHeaders)
		if err != nil {
			return false, err
		}
		lines = updated
		changed = changed || headersChanged
	}

	if !changed {
		return false, nil
	}

	if err := os.WriteFile(vhostConfigPath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return false, apperr.Wrap(apperr.CodeConfig, "failed to write vhost security configuration", err)
	}
	return true, nil
}

func appendBlock(lines, block []string) []string {
	out := append([]string{}, lines...)
	if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
		out = append(out, "")
	}
	out = append(out, block...)
	return out
}

func replaceBlock(lines []string, start, end int, block []string) []string {
	updated := append([]string{}, lines[:start]...)
	updated = append(updated, block...)
	updated = append(updated, lines[end+1:]...)
	return updated
}

func removeBlock(lines []string, start, end int) []string {
	updated := append([]string{}, lines[:start]...)
	updated = append(updated, lines[end+1:]...)
	return updated
}

func buildVHostOWASPBlock() []string {
	return []string{
		"module mod_security {",
		formatDirectiveLine("  ", "ls_enabled", "1"),
		"}",
	}
}

func upsertVHostOWASPBlock(lines []string, enabled bool) ([]string, bool, error) {
	start, end := findBlockByKey(lines, "module mod_security")
	if enabled {
		block := buildVHostOWASPBlock()
		if start < 0 || end < 0 {
			return appendBlock(lines, block), true, nil
		}
		current := append([]string{}, lines[start:end+1]...)
		if sameStringSlice(current, block) {
			return lines, false, nil
		}
		return replaceBlock(lines, start, end, block), true, nil
	}

	if start < 0 || end < 0 {
		return lines, false, nil
	}
	return removeBlock(lines, start, end), true, nil
}

func buildVHostRecaptchaBlock(recaptchaType, reqLimit int) []string {
	return []string{
		"lsrecaptcha  {",
		formatDirectiveLine("  ", "enabled", "1"),
		formatDirectiveLine("  ", "type", fmt.Sprintf("%d", recaptchaType)),
		formatDirectiveLine("  ", "regConnLimit", fmt.Sprintf("%d", reqLimit)),
		"}",
	}
}

func upsertVHostRecaptchaBlock(lines []string, enabled bool, recaptchaType, reqLimit int) ([]string, bool, error) {
	start, end := findBlockByKey(lines, "lsrecaptcha")
	if enabled {
		block := buildVHostRecaptchaBlock(recaptchaType, reqLimit)
		if start < 0 || end < 0 {
			return appendBlock(lines, block), true, nil
		}
		current := append([]string{}, lines[start:end+1]...)
		if sameStringSlice(current, block) {
			return lines, false, nil
		}
		return replaceBlock(lines, start, end, block), true, nil
	}

	if start < 0 || end < 0 {
		return lines, false, nil
	}
	return removeBlock(lines, start, end), true, nil
}

func upsertVHostNamespaceDirective(lines []string, enabled bool) ([]string, bool) {
	if enabled {
		return upsertDirective(lines, "namespace", "2")
	}
	return removeDirective(lines, "namespace")
}

func findExtraHeadersBlock(lines []string) (int, int, string) {
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "extraHeaders") || !strings.Contains(trimmed, "<<<") {
			continue
		}
		parts := strings.SplitN(trimmed, "<<<", 2)
		token := strings.TrimSpace(parts[1])
		if token == "" {
			token = "END_extraHeaders"
		}
		for j := i + 1; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) == token {
				return i, j, token
			}
		}
		return i, -1, token
	}
	return -1, -1, ""
}

func normalizeHeaderLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	seen := map[string]struct{}{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func ensureRootContextExtraHeaders(lines []string, requiredHeaders []string) ([]string, bool, error) {
	start, end := findBlockByKey(lines, "context /")
	if start < 0 || end < 0 {
		return nil, false, apperr.New(apperr.CodeValidation, "context / block not found in vhost config; cannot apply HSTS/security headers")
	}

	body := append([]string{}, lines[start+1:end]...)
	headerStart, headerEnd, _ := findExtraHeadersBlock(body)

	if headerStart >= 0 && headerEnd < 0 {
		return nil, false, apperr.New(apperr.CodeConfig, "malformed extraHeaders block in context /")
	}

	if headerStart < 0 {
		newBlock := []string{
			formatDirectiveLine("  ", "extraHeaders", "<<<END_extraHeaders"),
		}
		newBlock = append(newBlock, requiredHeaders...)
		newBlock = append(newBlock, "  END_extraHeaders")
		if len(body) > 0 && strings.TrimSpace(body[len(body)-1]) != "" {
			body = append(body, "")
		}
		body = append(body, newBlock...)

		newContext := []string{lines[start]}
		newContext = append(newContext, body...)
		newContext = append(newContext, lines[end])
		return replaceBlock(lines, start, end, newContext), true, nil
	}

	existingHeaders := normalizeHeaderLines(body[headerStart+1 : headerEnd])
	merged := append([]string{}, existingHeaders...)
	seen := map[string]struct{}{}
	for _, line := range existingHeaders {
		seen[strings.ToLower(strings.TrimSpace(line))] = struct{}{}
	}
	for _, header := range requiredHeaders {
		key := strings.ToLower(strings.TrimSpace(header))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, header)
	}

	if sameStringSlice(existingHeaders, merged) {
		return lines, false, nil
	}

	newHeadersBlock := []string{body[headerStart]}
	newHeadersBlock = append(newHeadersBlock, merged...)
	newHeadersBlock = append(newHeadersBlock, body[headerEnd])
	updatedBody := replaceBlock(body, headerStart, headerEnd, newHeadersBlock)

	newContext := []string{lines[start]}
	newContext = append(newContext, updatedBody...)
	newContext = append(newContext, lines[end])
	return replaceBlock(lines, start, end, newContext), true, nil
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

func letsEncryptCertPaths(domain string) (string, string) {
	liveDir := path.Join(letsencryptLiveRoot, strings.TrimSpace(strings.ToLower(domain)))
	return path.Join(liveDir, "fullchain.pem"), path.Join(liveDir, "privkey.pem")
}

func certbotPackagesFor(pm platform.PackageManager) []string {
	switch pm {
	case platform.PackageManagerAPT, platform.PackageManagerYUM, platform.PackageManagerDNF:
		return []string{"certbot"}
	default:
		return nil
	}
}

func (s SiteService) ensureCertbotAvailable(ctx context.Context, info platform.Info) error {
	if _, err := s.runner.Run(ctx, "sh", "-c", "command -v certbot >/dev/null 2>&1"); err == nil {
		return nil
	}

	pkgs := certbotPackagesFor(info.PackageManager)
	if len(pkgs) == 0 {
		return apperr.New(apperr.CodePlatform, fmt.Sprintf("certbot auto-install is unsupported for package manager %s", info.PackageManager))
	}

	s.console.Bullet("Installing certbot package")
	installer := platform.NewPackageInstaller(s.runner, info)
	if err := installer.Install(ctx, pkgs...); err != nil {
		return apperr.Wrap(apperr.CodeCommand, "failed to install certbot", err)
	}

	if _, err := s.runner.Run(ctx, "sh", "-c", "command -v certbot >/dev/null 2>&1"); err != nil {
		return apperr.New(apperr.CodeCommand, "certbot installation completed but executable was not found in PATH")
	}
	return nil
}

func (s SiteService) issueLetsEncryptCertificate(ctx context.Context, info platform.Info, domain, webRoot string) (string, string, error) {
	if err := s.ensureCertbotAvailable(ctx, info); err != nil {
		return "", "", err
	}

	res, err := s.runner.Run(
		ctx,
		"certbot",
		"certonly",
		"--non-interactive",
		"--agree-tos",
		"--register-unsafely-without-email",
		"--keep-until-expiring",
		"--webroot",
		"-w", webRoot,
		"-d", domain,
	)
	if err != nil {
		detail := strings.TrimSpace(strings.Join([]string{res.Stdout, res.Stderr}, "\n"))
		if detail != "" {
			return "", "", apperr.Wrap(apperr.CodeCommand, "certbot certificate issuance failed: "+detail, err)
		}
		return "", "", apperr.Wrap(apperr.CodeCommand, "certbot certificate issuance failed", err)
	}

	certFile, keyFile := letsEncryptCertPaths(domain)
	if !fileExists(certFile) || !fileExists(keyFile) {
		return "", "", apperr.New(
			apperr.CodeCommand,
			fmt.Sprintf("certbot completed but expected certificate files were not found (cert=%s key=%s)", certFile, keyFile),
		)
	}

	return certFile, keyFile, nil
}

func findBlockByKey(lines []string, key string) (int, int) {
	start := -1
	depth := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if start == -1 {
			if !strings.HasPrefix(trimmed, key+" ") || !strings.Contains(trimmed, "{") {
				continue
			}
			start = i
			depth = strings.Count(line, "{") - strings.Count(line, "}")
			if depth <= 0 {
				depth = 1
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

func applyVHostSSLCertificate(vhostConfigPath, certFile, keyFile string) error {
	b, err := os.ReadFile(vhostConfigPath)
	if err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to read vhost config for SSL update", err)
	}
	cfg := string(b)
	lines := strings.Split(cfg, "\n")

	start, end := findBlockByKey(lines, "vhssl")
	if start < 0 || end < 0 {
		block := strings.Join([]string{
			"vhssl  {",
			formatDirectiveLine("  ", "keyFile", keyFile),
			formatDirectiveLine("  ", "certFile", certFile),
			"}",
		}, "\n")
		updated := strings.TrimRight(cfg, "\n")
		if updated != "" {
			updated += "\n\n"
		}
		updated += block + "\n"
		if err := os.WriteFile(vhostConfigPath, []byte(updated), 0o644); err != nil {
			return apperr.Wrap(apperr.CodeConfig, "failed to write vhost SSL config", err)
		}
		return nil
	}

	body := append([]string{}, lines[start+1:end]...)
	changed := false
	body, changedKey := upsertDirective(body, "keyFile", keyFile)
	changed = changed || changedKey
	body, changedCert := upsertDirective(body, "certFile", certFile)
	changed = changed || changedCert
	if !changed {
		return nil
	}

	newBlock := []string{lines[start]}
	newBlock = append(newBlock, body...)
	newBlock = append(newBlock, lines[end])

	updatedLines := append([]string{}, lines[:start]...)
	updatedLines = append(updatedLines, newBlock...)
	updatedLines = append(updatedLines, lines[end+1:]...)
	if err := os.WriteFile(vhostConfigPath, []byte(strings.Join(updatedLines, "\n")), 0o644); err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to write vhost SSL config", err)
	}
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

func (s SiteService) reloadOpenLiteSpeed(ctx context.Context) error {
	lswsCtrlPath := filepath.Join(s.lswsRoot, "bin", "lswsctrl")
	if !fileExists(lswsCtrlPath) {
		return apperr.New(apperr.CodeValidation, "lswsctrl not found; cannot reload OpenLiteSpeed")
	}

	if _, err := s.runner.Run(ctx, lswsCtrlPath, "reload"); err == nil {
		s.console.Bullet("OpenLiteSpeed reloaded: " + lswsCtrlPath + " reload")
		return nil
	}

	if _, err := s.runner.Run(ctx, lswsCtrlPath, "restart"); err == nil {
		s.console.Bullet("OpenLiteSpeed restarted: " + lswsCtrlPath + " restart")
		return nil
	}

	return apperr.New(apperr.CodeCommand, "lswsctrl reload/restart failed")
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

func (s SiteService) inheritOwnershipFromParent(paths ...string) error {
	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			continue
		}
		if err := inheritPathOwnershipFromParent(p); err != nil {
			return err
		}
	}
	return nil
}

func inheritPathOwnershipFromParent(path string) error {
	cleanPath := filepath.Clean(path)
	info, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return apperr.Wrap(apperr.CodeConfig, "failed to stat path for ownership alignment", err)
	}

	parentInfo, err := os.Stat(filepath.Dir(cleanPath))
	if err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to stat parent path for ownership alignment", err)
	}

	targetUID, targetGID, ok := fileOwnership(parentInfo)
	if !ok {
		return nil
	}

	currentUID, currentGID, hasCurrentOwner := fileOwnership(info)
	if hasCurrentOwner && currentUID == targetUID && currentGID == targetGID {
		if !info.IsDir() {
			return nil
		}
	}

	if !info.IsDir() {
		if err := chownPath(cleanPath, targetUID, targetGID); err != nil {
			return apperr.Wrap(apperr.CodeConfig, "failed to align ownership with parent", err)
		}
		return nil
	}

	return filepath.WalkDir(cleanPath, func(currentPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := chownPath(currentPath, targetUID, targetGID); err != nil {
			return err
		}
		return nil
	})
}

func (s SiteService) applyServerConfiguredOwnership(serverConfigPath string, paths ...string) error {
	userName, groupName, err := readServerUserGroup(serverConfigPath)
	if err != nil {
		return err
	}
	uid, gid, err := lookupUserGroupIDs(userName, groupName)
	if err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to resolve OpenLiteSpeed user/group IDs", err)
	}
	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			continue
		}
		if err := applyOwnershipRecursive(p, uid, gid); err != nil {
			return err
		}
	}
	return nil
}

func readServerUserGroup(serverConfigPath string) (string, string, error) {
	b, err := os.ReadFile(serverConfigPath)
	if err != nil {
		return "", "", apperr.Wrap(apperr.CodeConfig, "failed to read OpenLiteSpeed server config", err)
	}
	cfg := string(b)
	userName := firstDirectiveValue(cfg, "user")
	if strings.TrimSpace(userName) == "" {
		return "", "", apperr.New(apperr.CodeConfig, "missing `user` directive in OpenLiteSpeed server config")
	}
	groupName := firstDirectiveValue(cfg, "group")
	return userName, groupName, nil
}

func firstDirectiveValue(cfg, key string) string {
	for _, line := range strings.Split(cfg, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}
		if strings.EqualFold(fields[0], key) {
			return fields[1]
		}
	}
	return ""
}

func applyOwnershipRecursive(root string, uid, gid int) error {
	root = filepath.Clean(root)
	if !fileExists(root) {
		return nil
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := chownPath(path, uid, gid); err != nil {
			return err
		}
		return nil
	})
}

func installWordPress(docRoot string) error {
	tmp, err := os.CreateTemp("", "ols-wordpress-*.tar.gz")
	if err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to prepare temporary WordPress archive", err)
	}
	archivePath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(archivePath)
		return apperr.Wrap(apperr.CodeConfig, "failed to initialize temporary WordPress archive", err)
	}
	defer func() {
		_ = os.Remove(archivePath)
	}()

	if err := downloadFileWithSHA1Verification(wpArchiveURL, wpArchiveSHA1URL, archivePath); err != nil {
		return err
	}

	f, err := os.Open(archivePath)
	if err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to open verified WordPress archive", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
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

func ensureWordPressFiles(docRoot string) error {
	if !looksLikeWordPressDocRoot(docRoot) {
		if err := installWordPress(docRoot); err != nil {
			return err
		}
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

func (s SiteService) provisionWordPressInstall(ctx context.Context, domain, docRoot, phpVersion, installURL string) (*wpAdminAccess, error) {
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
		"--url="+installURL,
		"--title="+domain,
		"--admin_user="+adminUser,
		"--admin_password="+adminPassword,
		"--admin_email="+adminEmail,
		"--skip-email",
		"--allow-root",
	); err != nil {
		return nil, err
	}
	if err := s.ensureWordPressURLs(ctx, phpPath, wpCLIPath, docRoot, installURL); err != nil {
		return nil, err
	}

	if err := s.runWPCLI(ctx, phpPath, wpCLIPath,
		"plugin", "install", "litespeed-cache",
		"--activate",
		"--force",
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
		AdminURL:      installURL + "/wp-admin",
		AdminUser:     adminUser,
		AdminPassword: adminPassword,
		SecretsFile:   secretsPath,
	}, nil
}

func (s SiteService) ensureWordPressURLs(ctx context.Context, phpPath, wpCLIPath, docRoot, baseURL string) error {
	updates := [][]string{
		{"option", "update", "home", baseURL},
		{"option", "update", "siteurl", baseURL},
	}
	for _, update := range updates {
		args := append(update, "--path="+docRoot, "--allow-root")
		if err := s.runWPCLI(ctx, phpPath, wpCLIPath, args...); err != nil {
			return err
		}
	}
	return nil
}

func (s SiteService) installAndActivateLiteSpeedCachePlugin(ctx context.Context, phpVersion, docRoot string) error {
	if !fileExists(filepath.Join(docRoot, "wp-config.php")) {
		return apperr.New(
			apperr.CodeValidation,
			fmt.Sprintf("wp-config.php not found in %s; cannot install LiteSpeed Cache via wp-cli", docRoot),
		)
	}

	phpPath, err := s.resolvePHPCLIPath(phpVersion)
	if err != nil {
		return err
	}
	wpCLIPath := filepath.Join(docRoot, ".ols-wp-cli.phar")
	if err := ensureWPCLIPhar(wpCLIPath); err != nil {
		return err
	}
	defer func() {
		_ = os.Remove(wpCLIPath)
	}()

	if err := s.runWPCLI(ctx, phpPath, wpCLIPath,
		"plugin", "install", "litespeed-cache",
		"--activate",
		"--force",
		"--path="+docRoot,
		"--allow-root",
	); err != nil {
		return err
	}
	return nil
}

func wordPressBaseURL(domain string, secure bool) string {
	scheme := "http"
	if secure {
		scheme = "https"
	}
	return scheme + "://" + strings.TrimSpace(strings.ToLower(domain))
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

func (s SiteService) dropWordPressDatabase(ctx context.Context, domain string) error {
	dbName, dbUser := deriveWordPressDBIdentifiers(domain)
	sql := fmt.Sprintf(
		"DROP DATABASE IF EXISTS `%s`; DROP USER IF EXISTS '%s'@'localhost'; FLUSH PRIVILEGES;",
		dbName,
		escapeSQLString(dbUser),
	)

	tmp, err := os.CreateTemp("", "ols-wp-drop-sql-*.sql")
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

	return apperr.New(apperr.CodeCommand, "failed to drop WordPress database/user; ensure local root DB access is available")
}

func removeIfExists(path string) error {
	path = filepath.Clean(path)
	err := os.RemoveAll(path)
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return err
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

	if err := os.MkdirAll(filepath.Dir(wpCLIPath), 0o755); err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to prepare wp-cli target path", err)
	}
	if err := downloadFileWithSHA512Verification(wpCLIPharURL, wpCLIPharSHA512URL, wpCLIPath, 0o755); err != nil {
		return err
	}
	return nil
}

func (s SiteService) runWPCLI(ctx context.Context, phpPath, wpCLIPath string, args ...string) error {
	cmdArgs := append([]string{wpCLIPath}, args...)
	res, err := s.runner.Run(ctx, phpPath, cmdArgs...)
	if err != nil {
		msg := strings.TrimSpace(strings.Join([]string{sanitizeOutputSecrets(res.Stdout), sanitizeOutputSecrets(res.Stderr)}, "\n"))
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

func buildVHostDefinition(domain, siteRoot, vhostConfigPath string) string {
	return fmt.Sprintf(
		"virtualhost %s {\n  vhRoot                  %s/\n  configFile              %s\n  allowSymbolLink         1\n  enableScript            1\n  restrained              1\n}\n",
		domain,
		siteRoot,
		vhostConfigPath,
	)
}

func buildVHConfig(phpVersion string) string {
	return fmt.Sprintf("docRoot                   $VH_ROOT/html/\n\nerrorlog $VH_ROOT/logs/error.log {\n  useServer               0\n  logLevel                WARN\n  rollingSize             10M\n}\n\naccesslog $VH_ROOT/logs/access.log {\n  useServer               0\n  logFormat               \"%%h %%l %%u %%t \\\"%%r\\\" %%>s %%b\"\n  logHeaders              5\n  rollingSize             10M\n  keepDays                10\n  compressArchive         1\n}\n\nindex  {\n  useServer               0\n  indexFiles              index.php, index.html\n}\n\ncontext / {\n  type                    null\n  location                $DOC_ROOT/\n  allowBrowse             1\n}\n\nextprocessor lsphp%s {\n  type                    lsapi\n  address                 uds://tmp/lshttpd/lsphp%s.sock\n  maxConns                35\n  env                     PHP_LSAPI_CHILDREN=35\n  env                     LSAPI_AVOID_FORK=200M\n  initTimeout             60\n  retryTimeout            0\n  persistConn             1\n  pcKeepAliveTimeout      1\n  respBuffer              0\n  autoStart               2\n  path                    /usr/local/lsws/lsphp%s/bin/lsphp\n  backlog                 100\n  instances               1\n  priority                0\n  memSoftLimit            0\n  memHardLimit            0\n  procSoftLimit           0\n  procHardLimit           0\n}\n\nscriptHandler  {\n  add                     lsapi:lsphp%s php\n}\n\nrewrite  {\n  enable                  1\n  autoLoadHtaccess        1\n}\n", phpVersion, phpVersion, phpVersion, phpVersion)
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

func newHTTPGetRequest(url string) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeValidation, "invalid download URL", err)
	}
	req.Header.Set("User-Agent", defaultHTTPUserAgent)
	req.Header.Set("Accept", "*/*")
	return req, nil
}

func isRetryableHTTPStatus(status int) bool {
	return status == http.StatusTooManyRequests ||
		status == http.StatusRequestTimeout ||
		status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable ||
		status == http.StatusGatewayTimeout ||
		status >= 500
}

func parseRetryAfterDelay(header string) time.Duration {
	raw := strings.TrimSpace(header)
	if raw == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(raw); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	if t, err := http.ParseTime(raw); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
	}
	return 0
}

func retryDelayForAttempt(attempt int, retryAfterHeader string) time.Duration {
	if d := parseRetryAfterDelay(retryAfterHeader); d > 0 {
		if d > defaultHTTPRetryMaxDelay {
			return defaultHTTPRetryMaxDelay
		}
		return d
	}
	delay := defaultHTTPRetryMinDelay
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= defaultHTTPRetryMaxDelay {
			return defaultHTTPRetryMaxDelay
		}
	}
	if delay > defaultHTTPRetryMaxDelay {
		return defaultHTTPRetryMaxDelay
	}
	return delay
}

func downloadText(url string) (string, error) {
	client := &http.Client{Timeout: 2 * time.Minute}
	var lastErr error
	for attempt := 1; attempt <= defaultHTTPRetryAttempts; attempt++ {
		req, err := newHTTPGetRequest(url)
		if err != nil {
			return "", err
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = apperr.Wrap(apperr.CodeCommand, "failed to download remote text", err)
			if attempt < defaultHTTPRetryAttempts {
				time.Sleep(retryDelayForAttempt(attempt, ""))
				continue
			}
			return "", lastErr
		}

		if resp.StatusCode != http.StatusOK {
			statusCode := resp.StatusCode
			retryAfter := resp.Header.Get("Retry-After")
			retryable := isRetryableHTTPStatus(statusCode)
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			lastErr = apperr.New(apperr.CodeCommand, fmt.Sprintf("failed to download remote text: http %d", statusCode))
			if retryable && attempt < defaultHTTPRetryAttempts {
				time.Sleep(retryDelayForAttempt(attempt, retryAfter))
				continue
			}
			return "", lastErr
		}

		b, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
		resp.Body.Close()
		if err != nil {
			lastErr = apperr.Wrap(apperr.CodeCommand, "failed to read remote text", err)
			if attempt < defaultHTTPRetryAttempts {
				time.Sleep(retryDelayForAttempt(attempt, ""))
				continue
			}
			return "", lastErr
		}
		return strings.TrimSpace(string(b)), nil
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", apperr.New(apperr.CodeCommand, "failed to download remote text")
}

func parseExpectedHexDigest(raw string, minLen, maxLen int) (string, error) {
	match := hexDigestPattern.FindString(strings.TrimSpace(raw))
	if match == "" {
		return "", apperr.New(apperr.CodeValidation, "no digest value found in checksum response")
	}
	digest := strings.ToLower(strings.TrimSpace(match))
	if len(digest) < minLen || len(digest) > maxLen {
		return "", apperr.New(apperr.CodeValidation, "invalid checksum length")
	}
	return digest, nil
}

func computeFileSHA1(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", apperr.Wrap(apperr.CodeConfig, "failed to open file for sha1", err)
	}
	defer f.Close()

	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", apperr.Wrap(apperr.CodeConfig, "failed to hash file with sha1", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func computeFileSHA512(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", apperr.Wrap(apperr.CodeConfig, "failed to open file for sha512", err)
	}
	defer f.Close()

	h := sha512.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", apperr.Wrap(apperr.CodeConfig, "failed to hash file with sha512", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func downloadToFile(url, destPath string, mode os.FileMode) error {
	client := &http.Client{Timeout: 2 * time.Minute}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to create destination directory", err)
	}

	var lastErr error
	for attempt := 1; attempt <= defaultHTTPRetryAttempts; attempt++ {
		req, err := newHTTPGetRequest(url)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = apperr.Wrap(apperr.CodeCommand, "failed to download file", err)
			if attempt < defaultHTTPRetryAttempts {
				time.Sleep(retryDelayForAttempt(attempt, ""))
				continue
			}
			return lastErr
		}

		if resp.StatusCode != http.StatusOK {
			statusCode := resp.StatusCode
			retryAfter := resp.Header.Get("Retry-After")
			retryable := isRetryableHTTPStatus(statusCode)
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			lastErr = apperr.New(apperr.CodeCommand, fmt.Sprintf("failed to download file: http %d", statusCode))
			if retryable && attempt < defaultHTTPRetryAttempts {
				time.Sleep(retryDelayForAttempt(attempt, retryAfter))
				continue
			}
			return lastErr
		}

		tmp, err := os.CreateTemp(filepath.Dir(destPath), "ols-download-*")
		if err != nil {
			resp.Body.Close()
			return apperr.Wrap(apperr.CodeConfig, "failed to create temporary download file", err)
		}
		tmpPath := tmp.Name()

		_, copyErr := io.Copy(tmp, io.LimitReader(resp.Body, 256*1024*1024))
		resp.Body.Close()
		closeErr := tmp.Close()

		if copyErr != nil {
			_ = os.Remove(tmpPath)
			lastErr = apperr.Wrap(apperr.CodeConfig, "failed to write downloaded file", copyErr)
			if attempt < defaultHTTPRetryAttempts {
				time.Sleep(retryDelayForAttempt(attempt, ""))
				continue
			}
			return lastErr
		}
		if closeErr != nil {
			_ = os.Remove(tmpPath)
			lastErr = apperr.Wrap(apperr.CodeConfig, "failed to finalize downloaded file", closeErr)
			if attempt < defaultHTTPRetryAttempts {
				time.Sleep(retryDelayForAttempt(attempt, ""))
				continue
			}
			return lastErr
		}
		if err := os.Chmod(tmpPath, mode); err != nil {
			_ = os.Remove(tmpPath)
			return apperr.Wrap(apperr.CodeConfig, "failed to set downloaded file permissions", err)
		}
		if err := os.Rename(tmpPath, destPath); err != nil {
			_ = os.Remove(tmpPath)
			return apperr.Wrap(apperr.CodeConfig, "failed to move downloaded file into place", err)
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return apperr.New(apperr.CodeCommand, "failed to download file")
}

func downloadFileWithSHA1Verification(url, checksumURL, destPath string) error {
	expectedRaw, err := downloadText(checksumURL)
	if err != nil {
		return apperr.Wrap(apperr.CodeCommand, "failed to download WordPress checksum", err)
	}
	expected, err := parseExpectedHexDigest(expectedRaw, 40, 40)
	if err != nil {
		return apperr.Wrap(apperr.CodeValidation, "invalid WordPress checksum response", err)
	}

	if err := downloadToFile(url, destPath, 0o644); err != nil {
		return err
	}
	actual, err := computeFileSHA1(destPath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(actual, expected) {
		return apperr.New(apperr.CodeCommand, "WordPress archive checksum verification failed")
	}
	return nil
}

func downloadFileWithSHA512Verification(url, checksumURL, destPath string, mode os.FileMode) error {
	expectedRaw, err := downloadText(checksumURL)
	if err != nil {
		return apperr.Wrap(apperr.CodeCommand, "failed to download wp-cli checksum", err)
	}
	expected, err := parseExpectedHexDigest(expectedRaw, 128, 128)
	if err != nil {
		return apperr.Wrap(apperr.CodeValidation, "invalid wp-cli checksum response", err)
	}

	if err := downloadToFile(url, destPath, mode); err != nil {
		return err
	}
	actual, err := computeFileSHA512(destPath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(actual, expected) {
		return apperr.New(apperr.CodeCommand, "wp-cli phar checksum verification failed")
	}
	return nil
}

func sanitizeOutputSecrets(content string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	res := content
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(--admin_password=)(\S+)`),
		regexp.MustCompile(`(?i)(--dbpass=)(\S+)`),
		regexp.MustCompile(`(?i)(password\s*[:=]\s*)(\S+)`),
	}
	for _, p := range patterns {
		res = p.ReplaceAllString(res, `${1}<redacted>`)
	}
	return res
}
