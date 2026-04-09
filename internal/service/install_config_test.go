package service

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ols/ols-cli/internal/platform"
	"github.com/ols/ols-cli/internal/ui"
)

func TestResolveInstallPlanDefaults(t *testing.T) {
	plan, err := resolveInstallPlan(
		InstallOptions{ConfigPath: filepath.Join(t.TempDir(), "install.json")},
		platform.Info{PackageManager: platform.PackageManagerAPT},
		"/usr/local/lsws",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plan.PHPVersion != "85" {
		t.Fatalf("expected default php 85, got %s", plan.PHPVersion)
	}
	if plan.DatabaseEngine != "mariadb" {
		t.Fatalf("expected default database mariadb, got %s", plan.DatabaseEngine)
	}
	if plan.DatabasePackage != "mariadb-server" {
		t.Fatalf("expected mariadb-server package, got %s", plan.DatabasePackage)
	}
	if !plan.ConfigureListeners {
		t.Fatalf("expected listeners enabled by default")
	}
	if plan.HTTPPort != 80 || plan.HTTPSPort != 443 {
		t.Fatalf("expected default ports 80/443, got %d/%d", plan.HTTPPort, plan.HTTPSPort)
	}
	if plan.OWASPCRSVersion != defaultOWASPCRSVersion {
		t.Fatalf("expected default owasp crs version %s, got %s", defaultOWASPCRSVersion, plan.OWASPCRSVersion)
	}
	if plan.VHRecaptchaType != defaultVHRecaptchaType {
		t.Fatalf("expected default vh recaptcha type %d, got %d", defaultVHRecaptchaType, plan.VHRecaptchaType)
	}
	if plan.VHRecaptchaReqLimit != defaultVHRecaptchaReqLimit {
		t.Fatalf("expected default vh recaptcha request limit %d, got %d", defaultVHRecaptchaReqLimit, plan.VHRecaptchaReqLimit)
	}
}

func TestResolveInstallPlanOverrides(t *testing.T) {
	disabled := false
	plan, err := resolveInstallPlan(
		InstallOptions{
			ConfigPath:         filepath.Join(t.TempDir(), "install.json"),
			PHPVersion:         "84",
			DatabaseEngine:     "mysql",
			ConfigureListeners: &disabled,
			HTTPPort:           8080,
			HTTPSPort:          8443,
			SSLCertFile:        "/tmp/custom.crt",
			SSLKeyFile:         "/tmp/custom.key",
		},
		platform.Info{PackageManager: platform.PackageManagerDNF},
		"/usr/local/lsws",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plan.PHPVersion != "84" {
		t.Fatalf("expected php 84, got %s", plan.PHPVersion)
	}
	if plan.DatabaseEngine != "mysql" {
		t.Fatalf("expected mysql, got %s", plan.DatabaseEngine)
	}
	if plan.DatabasePackage != "mysql-server" {
		t.Fatalf("expected mysql-server, got %s", plan.DatabasePackage)
	}
	if plan.ConfigureListeners {
		t.Fatalf("expected listeners disabled")
	}
	if plan.HTTPPort != 8080 || plan.HTTPSPort != 8443 {
		t.Fatalf("expected ports 8080/8443, got %d/%d", plan.HTTPPort, plan.HTTPSPort)
	}
}

func TestLoadRuntimeInstallConfigIncludesOWASPCRSVersion(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "install.json")
	content := `{
  "php_version": "85",
  "database": "mariadb",
  "configure_listeners": true,
  "http_port": 80,
  "https_port": 443,
  "ssl_cert_file": "/usr/local/lsws/admin/conf/webadmin.crt",
  "ssl_key_file": "/usr/local/lsws/admin/conf/webadmin.key",
  "owasp_crs_version": "4.22.0",
  "vh_recaptcha_type": 2,
  "vh_recaptcha_reg_conn_limit": 650
}`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, _, err := loadRuntimeInstallConfig(configPath, "/usr/local/lsws")
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	if cfg.OWASPCRSVersion != "4.22.0" {
		t.Fatalf("expected owasp_crs_version 4.22.0, got %s", cfg.OWASPCRSVersion)
	}
	if cfg.VHRecaptchaType != 2 {
		t.Fatalf("expected vh_recaptcha_type 2, got %d", cfg.VHRecaptchaType)
	}
	if cfg.VHRecaptchaReqLimit != 650 {
		t.Fatalf("expected vh_recaptcha_reg_conn_limit 650, got %d", cfg.VHRecaptchaReqLimit)
	}
}

func TestResolveInstallPlanDatabaseNone(t *testing.T) {
	plan, err := resolveInstallPlan(
		InstallOptions{ConfigPath: filepath.Join(t.TempDir(), "install.json"), DatabaseEngine: "none"},
		platform.Info{PackageManager: platform.PackageManagerAPT},
		"/usr/local/lsws",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.DatabasePackage != "" {
		t.Fatalf("expected empty package for database none, got %q", plan.DatabasePackage)
	}
}

func TestResolveInstallPlanInvalidDatabase(t *testing.T) {
	_, err := resolveInstallPlan(
		InstallOptions{ConfigPath: filepath.Join(t.TempDir(), "install.json"), DatabaseEngine: "postgres"},
		platform.Info{PackageManager: platform.PackageManagerAPT},
		"/usr/local/lsws",
	)
	if err == nil {
		t.Fatal("expected invalid database engine error")
	}
}

func TestResolveInstallPlanRejectsUnsafeSSLPaths(t *testing.T) {
	_, err := resolveInstallPlan(
		InstallOptions{
			ConfigPath:     filepath.Join(t.TempDir(), "install.json"),
			SSLCertFile:    "/etc/ssl/certs/server.crt\nmalicious 1",
			SSLKeyFile:     "/etc/ssl/private/server.key",
			HTTPPort:       80,
			HTTPSPort:      443,
			PHPVersion:     "85",
			DatabaseEngine: "mariadb",
		},
		platform.Info{PackageManager: platform.PackageManagerAPT},
		"/usr/local/lsws",
	)
	if err == nil {
		t.Fatal("expected unsafe ssl path validation error")
	}
}

func TestInstallRuntimeDryRunIncludesResolvedPlan(t *testing.T) {
	r := &fakeRunner{}
	svc := NewSiteService(
		fakeDetector{info: platform.Info{ID: "ubuntu", Family: platform.FamilyDebian, PackageManager: platform.PackageManagerAPT, VersionID: "24.04"}},
		r,
		ui.NewStyledConsole(&bytes.Buffer{}),
	)

	err := svc.InstallRuntime(context.Background(), InstallOptions{DryRun: true, DatabaseEngine: "mysql", ConfigPath: filepath.Join(t.TempDir(), "install.json")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.calls) != 0 {
		t.Fatalf("expected no runner calls in dry-run, got %d", len(r.calls))
	}
}
