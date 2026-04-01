package service

import (
	"bytes"
	"context"
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

