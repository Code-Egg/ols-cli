package service

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ols/ols-cli/internal/platform"
	"github.com/ols/ols-cli/internal/runner"
	"github.com/ols/ols-cli/internal/ui"
)

type fakeDetector struct {
	info platform.Info
	err  error
}

func (f fakeDetector) Detect(_ context.Context) (platform.Info, error) {
	return f.info, f.err
}

type fakeRunner struct {
	calls [][]string
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) (runner.Result, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	return runner.Result{}, nil
}

func TestNormalizePHPVersion(t *testing.T) {
	v, err := NormalizePHPVersion("8.2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != "82" {
		t.Fatalf("expected 82, got %s", v)
	}
	if _, err := NormalizePHPVersion("8.9"); err == nil {
		t.Fatal("expected unsupported version error")
	}
}

func TestValidateDomain(t *testing.T) {
	if err := ValidateDomain("example.com"); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if err := ValidateDomain("bad_domain"); err == nil {
		t.Fatal("expected invalid domain error")
	}
}

func TestCreateSiteDryRun(t *testing.T) {
	var out bytes.Buffer
	console := ui.NewStyledConsole(&out)
	r := &fakeRunner{}
	svc := NewSiteService(
		fakeDetector{info: platform.Info{ID: "ubuntu", Family: platform.FamilyDebian, PackageManager: platform.PackageManagerAPT, VersionID: "22.04"}},
		r,
		console,
	)

	err := svc.CreateSite(context.Background(), CreateSiteOptions{
		Domain:        "example.com",
		WithWordPress: true,
		WithLE:        true,
		PHPVersion:    "82",
		DryRun:        true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.calls) != 0 {
		t.Fatalf("expected no runner calls in dry-run, got %d", len(r.calls))
	}
}

func TestInstallRuntimeDryRun(t *testing.T) {
	var out bytes.Buffer
	console := ui.NewStyledConsole(&out)
	r := &fakeRunner{}
	svc := NewSiteService(
		fakeDetector{info: platform.Info{ID: "ubuntu", Family: platform.FamilyDebian, PackageManager: platform.PackageManagerAPT, VersionID: "24.04"}},
		r,
		console,
	)

	err := svc.InstallRuntime(context.Background(), InstallOptions{PHPVersion: "82", DryRun: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.calls) != 0 {
		t.Fatalf("expected no runner calls in dry-run, got %d", len(r.calls))
	}
	if !strings.Contains(out.String(), "Install runtime") {
		t.Fatalf("expected install output, got: %s", out.String())
	}
}

func TestCreateSiteCreatesVHostAndDocRoot(t *testing.T) {
	var out bytes.Buffer
	console := ui.NewStyledConsole(&out)
	r := &fakeRunner{}

	base := t.TempDir()
	lswsRoot := filepath.Join(base, "lsws")
	webRoot := filepath.Join(base, "www")

	if err := os.MkdirAll(filepath.Join(lswsRoot, "bin"), 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(lswsRoot, "lsphp82", "bin"), 0o755); err != nil {
		t.Fatalf("mkdir php bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lswsRoot, "bin", "lswsctrl"), []byte("stub"), 0o755); err != nil {
		t.Fatalf("write lswsctrl: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lswsRoot, "lsphp82", "bin", "lsphp"), []byte("stub"), 0o755); err != nil {
		t.Fatalf("write lsphp: %v", err)
	}

	svc := NewSiteServiceWithPaths(
		fakeDetector{info: platform.Info{ID: "ubuntu", Family: platform.FamilyDebian, PackageManager: platform.PackageManagerAPT, VersionID: "24.04"}},
		r,
		console,
		lswsRoot,
		webRoot,
	)

	err := svc.CreateSite(context.Background(), CreateSiteOptions{
		Domain:     "example.com",
		PHPVersion: "82",
		DryRun:     false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	docRoot := filepath.Join(webRoot, "example.com", "html")
	if _, err := os.Stat(filepath.Join(docRoot, "index.php")); err != nil {
		t.Fatalf("expected starter index.php: %v", err)
	}

	vhostDir := filepath.Join(lswsRoot, "conf", "vhosts", "example.com")
	if _, err := os.Stat(filepath.Join(vhostDir, "vhconf.conf")); err != nil {
		t.Fatalf("expected vhconf.conf: %v", err)
	}
	if _, err := os.Stat(filepath.Join(vhostDir, "vhost.conf")); err != nil {
		t.Fatalf("expected vhost.conf: %v", err)
	}
}
