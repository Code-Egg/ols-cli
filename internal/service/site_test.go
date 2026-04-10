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
	calls    [][]string
	failures map[string]error
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) (runner.Result, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	if f.failures != nil {
		if err, ok := f.failures[name+" "+strings.Join(args, " ")]; ok {
			return runner.Result{}, err
		}
	}
	return runner.Result{}, nil
}

func boolPtr(v bool) *bool {
	return &v
}

func ptr[T any](v T) *T {
	return &v
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
		Domain:           "example.com",
		WithWordPress:    true,
		WithLE:           true,
		PHPVersion:       "82",
		NamespaceEnabled: ptr(true),
		DryRun:           true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.calls) != 0 {
		t.Fatalf("expected no runner calls in dry-run, got %d", len(r.calls))
	}
	if !strings.Contains(out.String(), "finish WordPress installation via wp-cli") {
		t.Fatalf("expected wp-cli plan output, got: %s", out.String())
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

	err := svc.InstallRuntime(context.Background(), InstallOptions{PHPVersion: "82", DryRun: true, ConfigPath: filepath.Join(t.TempDir(), "install.json")})
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

	logsRoot := filepath.Join(webRoot, "example.com", "logs")
	if _, err := os.Stat(logsRoot); err != nil {
		t.Fatalf("expected logs directory: %v", err)
	}

	vhostDir := filepath.Join(lswsRoot, "conf", "vhosts", "example.com")
	if _, err := os.Stat(filepath.Join(vhostDir, "vhconf.conf")); err != nil {
		t.Fatalf("expected vhconf.conf: %v", err)
	}
	if _, err := os.Stat(filepath.Join(vhostDir, "vhost.conf")); err != nil {
		t.Fatalf("expected vhost.conf: %v", err)
	}
	vhConf, err := os.ReadFile(filepath.Join(vhostDir, "vhconf.conf"))
	if err != nil {
		t.Fatalf("read vhconf: %v", err)
	}
	vhContent := string(vhConf)
	if !strings.Contains(vhContent, "errorlog $VH_ROOT/logs/error.log") {
		t.Fatalf("expected vhost error log under $VH_ROOT/logs, got: %s", vhContent)
	}
	if !strings.Contains(vhContent, "accesslog $VH_ROOT/logs/access.log") {
		t.Fatalf("expected vhost access log under $VH_ROOT/logs, got: %s", vhContent)
	}

	cfg, err := os.ReadFile(filepath.Join(lswsRoot, "conf", "httpd_config.conf"))
	if err != nil {
		t.Fatalf("read server config: %v", err)
	}
	cfgS := string(cfg)
	if !strings.Contains(cfgS, "virtualhost example.com {") {
		t.Fatalf("expected virtualhost block in server config: %s", cfgS)
	}
	if !strings.Contains(cfgS, "map                     example.com www.example.com, example.com") {
		t.Fatalf("expected listener map with www alias in server config: %s", cfgS)
	}

	reloadCalled := false
	for _, call := range r.calls {
		if len(call) == 2 && filepath.Base(call[0]) == "lswsctrl" && call[1] == "reload" {
			reloadCalled = true
			break
		}
	}
	if !reloadCalled {
		t.Fatalf("expected lswsctrl reload command, got calls: %#v", r.calls)
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

func TestUpdateSiteSecurityOnlyWithoutPHP(t *testing.T) {
	var out bytes.Buffer
	console := ui.NewStyledConsole(&out)
	r := &fakeRunner{}

	base := t.TempDir()
	lswsRoot := filepath.Join(base, "lsws")
	webRoot := filepath.Join(base, "www")
	vhostDir := filepath.Join(lswsRoot, "conf", "vhosts", "example.com")

	if err := os.MkdirAll(vhostDir, 0o755); err != nil {
		t.Fatalf("mkdir vhost: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(lswsRoot, "conf"), 0o755); err != nil {
		t.Fatalf("mkdir conf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lswsRoot, "conf", "httpd_config.conf"), []byte("listener Default {\n}\n"), 0o644); err != nil {
		t.Fatalf("write server config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vhostDir, "vhconf.conf"), []byte(buildVHConfig("85")), 0o644); err != nil {
		t.Fatalf("write vhconf: %v", err)
	}

	svc := NewSiteServiceWithPaths(
		fakeDetector{info: platform.Info{ID: "ubuntu", Family: platform.FamilyDebian, PackageManager: platform.PackageManagerAPT, VersionID: "24.04"}},
		r,
		console,
		lswsRoot,
		webRoot,
	)

	err := svc.UpdateSitePHP(context.Background(), UpdateSiteOptions{
		Domain:           "example.com",
		RecaptchaEnabled: boolPtr(true),
		NamespaceEnabled: ptr(true),
	})
	if err != nil {
		t.Fatalf("unexpected update error: %v", err)
	}
	if len(r.calls) != 0 {
		t.Fatalf("expected no command runner calls for security-only update, got: %#v", r.calls)
	}

	updated, err := os.ReadFile(filepath.Join(vhostDir, "vhconf.conf"))
	if err != nil {
		t.Fatalf("read vhconf: %v", err)
	}
	content := string(updated)
	if !strings.Contains(content, "lsrecaptcha") || !strings.Contains(content, "enabled") || !strings.Contains(content, "1") {
		t.Fatalf("expected recaptcha block enabled in vhconf, got: %s", content)
	}
	if !strings.Contains(content, "type                     1") {
		t.Fatalf("expected recaptcha type 1 in vhconf, got: %s", content)
	}
	if !strings.Contains(content, "regConnLimit             500") {
		t.Fatalf("expected recaptcha regConnLimit 500 in vhconf, got: %s", content)
	}
	if !strings.Contains(content, "namespace                2") {
		t.Fatalf("expected namespace directive enabled in vhconf, got: %s", content)
	}
}

func TestApplyVHostSecurityOptionsEnableAndDisable(t *testing.T) {
	vhostPath := filepath.Join(t.TempDir(), "vhconf.conf")
	if err := os.WriteFile(vhostPath, []byte(buildVHConfig("85")), 0o644); err != nil {
		t.Fatalf("write vhconf: %v", err)
	}

	changed, err := applyVHostSecurityOptions(vhostPath, vhostSecurityOptions{
		OWASPEnabled:      boolPtr(true),
		RecaptchaEnabled:  boolPtr(true),
		NamespaceEnabled:  boolPtr(true),
		EnableHSTSHeaders: true,
	})
	if err != nil {
		t.Fatalf("unexpected apply error: %v", err)
	}
	if !changed {
		t.Fatal("expected security config to change")
	}

	b, err := os.ReadFile(vhostPath)
	if err != nil {
		t.Fatalf("read vhconf: %v", err)
	}
	content := string(b)
	if !strings.Contains(content, "module mod_security") || !strings.Contains(content, "ls_enabled               1") {
		t.Fatalf("expected mod_security enabled block, got: %s", content)
	}
	if !strings.Contains(content, "lsrecaptcha") || !strings.Contains(content, "enabled") {
		t.Fatalf("expected recaptcha block, got: %s", content)
	}
	if !strings.Contains(content, "type                     1") || !strings.Contains(content, "regConnLimit             500") {
		t.Fatalf("expected recaptcha type/limit in block, got: %s", content)
	}
	if !strings.Contains(content, "namespace                2") {
		t.Fatalf("expected namespace directive enabled, got: %s", content)
	}
	if !strings.Contains(content, "extraHeaders") || !strings.Contains(content, "Strict-Transport-Security") {
		t.Fatalf("expected security headers block, got: %s", content)
	}

	changed, err = applyVHostSecurityOptions(vhostPath, vhostSecurityOptions{
		OWASPEnabled:     boolPtr(false),
		RecaptchaEnabled: boolPtr(false),
		NamespaceEnabled: boolPtr(false),
	})
	if err != nil {
		t.Fatalf("unexpected disable apply error: %v", err)
	}
	if !changed {
		t.Fatal("expected disable operation to change config")
	}

	b, err = os.ReadFile(vhostPath)
	if err != nil {
		t.Fatalf("read vhconf: %v", err)
	}
	content = string(b)
	if strings.Contains(content, "module mod_security") {
		t.Fatalf("expected mod_security block removed when disabled, got: %s", content)
	}
	if strings.Contains(content, "lsrecaptcha") {
		t.Fatalf("expected recaptcha block removed when disabled, got: %s", content)
	}
	if strings.Contains(content, "namespace") {
		t.Fatalf("expected namespace directive removed when disabled, got: %s", content)
	}
}

func TestEnsureServerSecurityDefaults(t *testing.T) {
	var out bytes.Buffer
	console := ui.NewStyledConsole(&out)
	r := &fakeRunner{}

	base := t.TempDir()
	lswsRoot := filepath.Join(base, "lsws")
	webRoot := filepath.Join(base, "www")
	serverConfigPath := filepath.Join(lswsRoot, "conf", "httpd_config.conf")

	if err := os.MkdirAll(filepath.Dir(serverConfigPath), 0o755); err != nil {
		t.Fatalf("mkdir conf dir: %v", err)
	}
	if err := os.WriteFile(serverConfigPath, []byte("listener Default {\n}\n"), 0o644); err != nil {
		t.Fatalf("write server config: %v", err)
	}

	svc := NewSiteServiceWithPaths(
		fakeDetector{info: platform.Info{ID: "ubuntu", Family: platform.FamilyDebian, PackageManager: platform.PackageManagerAPT, VersionID: "24.04"}},
		r,
		console,
		lswsRoot,
		webRoot,
	)

	if err := svc.ensureServerSecurityDefaults(serverConfigPath); err != nil {
		t.Fatalf("unexpected server defaults error: %v", err)
	}

	updated, err := os.ReadFile(serverConfigPath)
	if err != nil {
		t.Fatalf("read server config: %v", err)
	}
	content := string(updated)
	if !strings.Contains(content, "module mod_security") || !strings.Contains(content, "ls_enabled               0") {
		t.Fatalf("expected server mod_security defaults, got: %s", content)
	}
	if !strings.Contains(content, "lsrecaptcha") || !strings.Contains(content, "enabled                  1") || !strings.Contains(content, "type                     0") {
		t.Fatalf("expected server recaptcha defaults, got: %s", content)
	}
	if !strings.Contains(content, "namespace                1") {
		t.Fatalf("expected server namespace default namespace=1, got: %s", content)
	}
}

func TestDeriveWordPressDBIdentifiers(t *testing.T) {
	dbName, dbUser := deriveWordPressDBIdentifiers("Example-99.com")
	if dbName != "wp_example_99_com" {
		t.Fatalf("unexpected db name: %s", dbName)
	}
	if dbUser != "wpu_example_99_com" {
		t.Fatalf("unexpected db user: %s", dbUser)
	}
}

func TestGenerateSecurePasswordLength(t *testing.T) {
	password, err := generateSecurePassword(20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(password) != 20 {
		t.Fatalf("expected length 20, got %d", len(password))
	}
}

func TestUpsertINIValueReplacesExisting(t *testing.T) {
	in := "memory_limit = 128M\npost_max_size = 8M\n"
	out, changed := upsertINIValue(in, "memory_limit", "1024M")
	if !changed {
		t.Fatal("expected change when key exists")
	}
	if !strings.Contains(out, "memory_limit = 1024M") {
		t.Fatalf("expected updated memory_limit, got: %s", out)
	}
	if strings.Contains(out, "memory_limit = 128M") {
		t.Fatalf("expected old memory_limit removed, got: %s", out)
	}
}

func TestUpsertINIValueAppendsMissing(t *testing.T) {
	in := "post_max_size = 8M\n"
	out, changed := upsertINIValue(in, "upload_max_filesize", "1000M")
	if !changed {
		t.Fatal("expected change when key is missing")
	}
	if !strings.Contains(out, "upload_max_filesize = 1000M") {
		t.Fatalf("expected appended key, got: %s", out)
	}
}

func TestDiscoverLiteSpeedPHPINIPaths(t *testing.T) {
	root := t.TempDir()
	pathsToCreate := []string{
		filepath.Join(root, "lsphp81", "etc", "php", "8.1", "litespeed", "php.ini"),
		filepath.Join(root, "lsphp85", "etc", "php", "8.5", "litespeed", "php.ini"),
		filepath.Join(root, "lsphp84", "etc", "php.ini"),
	}
	for _, p := range pathsToCreate {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", p, err)
		}
		if err := os.WriteFile(p, []byte("memory_limit = 128M\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}

	paths, err := discoverLiteSpeedPHPINIPaths(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 3 {
		t.Fatalf("expected 3 php.ini files, got %d (%v)", len(paths), paths)
	}
}

func TestApplyPHPINISettingsFile(t *testing.T) {
	iniPath := filepath.Join(t.TempDir(), "php.ini")
	initial := "post_max_size = 8M\nupload_max_filesize = 2M\n"
	if err := os.WriteFile(iniPath, []byte(initial), 0o644); err != nil {
		t.Fatalf("write initial php.ini: %v", err)
	}

	err := applyPHPINISettingsFile(iniPath, []phpINISetting{
		{key: "post_max_size", value: "1000M"},
		{key: "upload_max_filesize", value: "1000M"},
		{key: "memory_limit", value: "1024M"},
		{key: "max_execution_time", value: "600"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	b, err := os.ReadFile(iniPath)
	if err != nil {
		t.Fatalf("read updated php.ini: %v", err)
	}
	content := string(b)
	checks := []string{
		"post_max_size = 1000M",
		"upload_max_filesize = 1000M",
		"memory_limit = 1024M",
		"max_execution_time = 600",
	}
	for _, c := range checks {
		if !strings.Contains(content, c) {
			t.Fatalf("expected %q in php.ini, got: %s", c, content)
		}
	}
}

func TestApplyVHostSSLCertificateAppendsBlock(t *testing.T) {
	p := filepath.Join(t.TempDir(), "vhconf.conf")
	initial := buildVHConfig("85")
	if err := os.WriteFile(p, []byte(initial), 0o644); err != nil {
		t.Fatalf("write initial vhconf: %v", err)
	}

	err := applyVHostSSLCertificate(p, "/etc/letsencrypt/live/example.com/fullchain.pem", "/etc/letsencrypt/live/example.com/privkey.pem")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read vhconf: %v", err)
	}
	content := string(b)
	if !strings.Contains(content, "vhssl  {") {
		t.Fatalf("expected vhssl block, got: %s", content)
	}
	if !strings.Contains(content, "certFile") || !strings.Contains(content, "fullchain.pem") {
		t.Fatalf("expected certFile directive, got: %s", content)
	}
	if !strings.Contains(content, "keyFile") || !strings.Contains(content, "privkey.pem") {
		t.Fatalf("expected keyFile directive, got: %s", content)
	}
}

func TestApplyVHostSSLCertificateUpdatesExistingBlock(t *testing.T) {
	p := filepath.Join(t.TempDir(), "vhconf.conf")
	initial := strings.Join([]string{
		"docRoot                   $VH_ROOT/html/",
		"",
		"vhssl  {",
		"  keyFile                 /tmp/old.key",
		"  certFile                /tmp/old.crt",
		"}",
		"",
	}, "\n")
	if err := os.WriteFile(p, []byte(initial), 0o644); err != nil {
		t.Fatalf("write initial vhconf: %v", err)
	}

	err := applyVHostSSLCertificate(p, "/etc/letsencrypt/live/example.com/fullchain.pem", "/etc/letsencrypt/live/example.com/privkey.pem")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read vhconf: %v", err)
	}
	content := string(b)
	if strings.Contains(content, "/tmp/old.key") || strings.Contains(content, "/tmp/old.crt") {
		t.Fatalf("expected old cert paths replaced, got: %s", content)
	}
	if !strings.Contains(content, "fullchain.pem") || !strings.Contains(content, "privkey.pem") {
		t.Fatalf("expected new cert paths present, got: %s", content)
	}
}

func TestIssueLetsEncryptCertificateSuccess(t *testing.T) {
	var out bytes.Buffer
	console := ui.NewStyledConsole(&out)
	r := &fakeRunner{}
	svc := NewSiteService(
		fakeDetector{info: platform.Info{ID: "ubuntu", Family: platform.FamilyDebian, PackageManager: platform.PackageManagerAPT, VersionID: "24.04"}},
		r,
		console,
	)

	oldRoot := letsencryptLiveRoot
	letsencryptLiveRoot = filepath.Join(t.TempDir(), "letsencrypt", "live")
	t.Cleanup(func() { letsencryptLiveRoot = oldRoot })

	certFile, keyFile := letsEncryptCertPaths("example.com")
	if err := os.MkdirAll(filepath.Dir(certFile), 0o755); err != nil {
		t.Fatalf("mkdir cert dir: %v", err)
	}
	if err := os.WriteFile(certFile, []byte("cert"), 0o644); err != nil {
		t.Fatalf("write cert file: %v", err)
	}
	if err := os.WriteFile(keyFile, []byte("key"), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	gotCert, gotKey, err := svc.issueLetsEncryptCertificate(context.Background(), platform.Info{PackageManager: platform.PackageManagerAPT}, "/var/www/example.com/html", "example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotCert != certFile || gotKey != keyFile {
		t.Fatalf("unexpected cert paths: cert=%s key=%s", gotCert, gotKey)
	}

	hasCertbot := false
	for _, call := range r.calls {
		if len(call) > 0 && call[0] == "certbot" {
			hasCertbot = true
			if !strings.Contains(strings.Join(call, " "), "-d example.com") {
				t.Fatalf("expected certbot args to include -d example.com, got: %#v", call)
			}
			break
		}
	}
	if !hasCertbot {
		t.Fatalf("expected certbot to be called, got calls: %#v", r.calls)
	}
}

func TestIssueLetsEncryptCertificateMultipleDomains(t *testing.T) {
	var out bytes.Buffer
	console := ui.NewStyledConsole(&out)
	r := &fakeRunner{}
	svc := NewSiteService(
		fakeDetector{info: platform.Info{ID: "ubuntu", Family: platform.FamilyDebian, PackageManager: platform.PackageManagerAPT, VersionID: "24.04"}},
		r,
		console,
	)

	oldRoot := letsencryptLiveRoot
	letsencryptLiveRoot = filepath.Join(t.TempDir(), "letsencrypt", "live")
	t.Cleanup(func() { letsencryptLiveRoot = oldRoot })

	certFile, keyFile := letsEncryptCertPaths("litespeedtech.club")
	if err := os.MkdirAll(filepath.Dir(certFile), 0o755); err != nil {
		t.Fatalf("mkdir cert dir: %v", err)
	}
	if err := os.WriteFile(certFile, []byte("cert"), 0o644); err != nil {
		t.Fatalf("write cert file: %v", err)
	}
	if err := os.WriteFile(keyFile, []byte("key"), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	gotCert, gotKey, err := svc.issueLetsEncryptCertificate(
		context.Background(),
		platform.Info{PackageManager: platform.PackageManagerAPT},
		"/var/www/litespeedtech.club/html",
		"litespeedtech.club",
		"www.litespeedtech.club",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotCert != certFile || gotKey != keyFile {
		t.Fatalf("unexpected cert paths: cert=%s key=%s", gotCert, gotKey)
	}

	found := false
	for _, call := range r.calls {
		if len(call) == 0 || call[0] != "certbot" {
			continue
		}
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "-d litespeedtech.club") && strings.Contains(joined, "-d www.litespeedtech.club") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected certbot args to include both domains, got calls: %#v", r.calls)
	}
}

func TestIsTopLevelSiteDomain(t *testing.T) {
	if !isTopLevelSiteDomain("litespeedtech.club") {
		t.Fatal("expected litespeedtech.club to be treated as top-level domain")
	}
	if !isTopLevelSiteDomain("example.com.tw") {
		t.Fatal("expected example.com.tw to be treated as top-level domain")
	}
	if isTopLevelSiteDomain("api.litespeedtech.club") {
		t.Fatal("expected api.litespeedtech.club to be treated as subdomain")
	}
}

func TestMappedListenerDomains(t *testing.T) {
	gotTop := mappedListenerDomains("litespeedtech.club")
	if len(gotTop) != 2 || gotTop[0] != "www.litespeedtech.club" || gotTop[1] != "litespeedtech.club" {
		t.Fatalf("unexpected mapped domains for top-level domain: %#v", gotTop)
	}

	gotSub := mappedListenerDomains("api.litespeedtech.club")
	if len(gotSub) != 1 || gotSub[0] != "api.litespeedtech.club" {
		t.Fatalf("unexpected mapped domains for subdomain: %#v", gotSub)
	}
}

func TestCertbotPackagesFor(t *testing.T) {
	if len(certbotPackagesFor(platform.PackageManagerAPT)) == 0 {
		t.Fatal("expected certbot package for apt")
	}
	if got := certbotPackagesFor(platform.PackageManager("unknown")); len(got) != 0 {
		t.Fatalf("expected no package for unknown package manager, got: %v", got)
	}
}
