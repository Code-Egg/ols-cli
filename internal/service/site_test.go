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
	if err := os.MkdirAll(filepath.Join(lswsRoot, "conf"), 0o755); err != nil {
		t.Fatalf("mkdir conf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lswsRoot, "bin", "lswsctrl"), []byte("stub"), 0o755); err != nil {
		t.Fatalf("write lswsctrl: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lswsRoot, "lsphp82", "bin", "lsphp"), []byte("stub"), 0o755); err != nil {
		t.Fatalf("write lsphp: %v", err)
	}
	serverCfg := "listener Default {\n  address                 *:80\n}\n"
	if err := os.WriteFile(filepath.Join(lswsRoot, "conf", "httpd_config.conf"), []byte(serverCfg), 0o644); err != nil {
		t.Fatalf("write server config: %v", err)
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

	cfg, err := os.ReadFile(filepath.Join(lswsRoot, "conf", "httpd_config.conf"))
	if err != nil {
		t.Fatalf("read server config: %v", err)
	}
	cfgS := string(cfg)
	if !strings.Contains(cfgS, "virtualhost example.com {") {
		t.Fatalf("expected virtualhost block in server config: %s", cfgS)
	}
	if !strings.Contains(cfgS, "map                     example.com example.com") {
		t.Fatalf("expected listener map in server config: %s", cfgS)
	}
}

func TestCreateSiteDuplicateDomainRejected(t *testing.T) {
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
	if err := os.MkdirAll(filepath.Join(lswsRoot, "conf", "vhosts", "example.com"), 0o755); err != nil {
		t.Fatalf("mkdir vhost: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lswsRoot, "bin", "lswsctrl"), []byte("stub"), 0o755); err != nil {
		t.Fatalf("write lswsctrl: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lswsRoot, "lsphp82", "bin", "lsphp"), []byte("stub"), 0o755); err != nil {
		t.Fatalf("write lsphp: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lswsRoot, "conf", "vhosts", "example.com", "vhconf.conf"), []byte("stub"), 0o644); err != nil {
		t.Fatalf("write vhconf: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(lswsRoot, "conf"), 0o755); err != nil {
		t.Fatalf("mkdir conf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lswsRoot, "conf", "httpd_config.conf"), []byte("listener Default {\n}\n"), 0o644); err != nil {
		t.Fatalf("write server config: %v", err)
	}

	svc := NewSiteServiceWithPaths(
		fakeDetector{info: platform.Info{ID: "ubuntu", Family: platform.FamilyDebian, PackageManager: platform.PackageManagerAPT, VersionID: "24.04"}},
		r,
		console,
		lswsRoot,
		webRoot,
	)

	err := svc.CreateSite(context.Background(), CreateSiteOptions{Domain: "example.com", PHPVersion: "82"})
	if err == nil {
		t.Fatal("expected duplicate domain error")
	}
}

func TestUpdateSitePHPSwitchesHandler(t *testing.T) {
	var out bytes.Buffer
	console := ui.NewStyledConsole(&out)
	r := &fakeRunner{}

	base := t.TempDir()
	lswsRoot := filepath.Join(base, "lsws")
	webRoot := filepath.Join(base, "www")
	vhostDir := filepath.Join(lswsRoot, "conf", "vhosts", "example.com")

	if err := os.MkdirAll(filepath.Join(lswsRoot, "bin"), 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(lswsRoot, "lsphp83", "bin"), 0o755); err != nil {
		t.Fatalf("mkdir php83 bin: %v", err)
	}
	if err := os.MkdirAll(vhostDir, 0o755); err != nil {
		t.Fatalf("mkdir vhost: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lswsRoot, "bin", "lswsctrl"), []byte("stub"), 0o755); err != nil {
		t.Fatalf("write lswsctrl: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lswsRoot, "lsphp83", "bin", "lsphp"), []byte("stub"), 0o755); err != nil {
		t.Fatalf("write lsphp83: %v", err)
	}
	origVh := buildVHConfig("82")
	if err := os.WriteFile(filepath.Join(vhostDir, "vhconf.conf"), []byte(origVh), 0o644); err != nil {
		t.Fatalf("write vhconf: %v", err)
	}

	svc := NewSiteServiceWithPaths(
		fakeDetector{info: platform.Info{ID: "ubuntu", Family: platform.FamilyDebian, PackageManager: platform.PackageManagerAPT, VersionID: "24.04"}},
		r,
		console,
		lswsRoot,
		webRoot,
	)

	err := svc.UpdateSitePHP(context.Background(), UpdateSiteOptions{Domain: "example.com", PHPVersion: "83", DryRun: false})
	if err != nil {
		t.Fatalf("unexpected update error: %v", err)
	}

	updated, err := os.ReadFile(filepath.Join(vhostDir, "vhconf.conf"))
	if err != nil {
		t.Fatalf("read vhconf: %v", err)
	}
	if strings.Contains(string(updated), "lsphp82") {
		t.Fatalf("expected old handler removed, got: %s", string(updated))
	}
	if !strings.Contains(string(updated), "lsphp83") {
		t.Fatalf("expected new handler present, got: %s", string(updated))
	}
}
