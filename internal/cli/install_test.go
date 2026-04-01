package cli

import (
	"context"
	"testing"

	"github.com/ols/ols-cli/internal/service"
)

type captureRuntimeInstaller struct {
	opts service.InstallOptions
}

func (c *captureRuntimeInstaller) InstallRuntime(_ context.Context, opts service.InstallOptions) error {
	c.opts = opts
	return nil
}

func TestInstallCmdParsesConfigFlags(t *testing.T) {
	installer := &captureRuntimeInstaller{}
	rootOpts := &rootOptions{}
	cmd := newInstallCmd(installer, rootOpts)
	cmd.SetArgs([]string{"--config", "/tmp/install.json", "--database", "mysql", "--http-port", "8080", "--https-port", "8443", "--ssl-cert", "/tmp/cert.pem", "--ssl-key", "/tmp/key.pem", "--php85"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}

	if installer.opts.PHPVersion != "85" {
		t.Fatalf("expected php 85, got %s", installer.opts.PHPVersion)
	}
	if installer.opts.ConfigPath != "/tmp/install.json" {
		t.Fatalf("expected config path override, got %s", installer.opts.ConfigPath)
	}
	if installer.opts.DatabaseEngine != "mysql" {
		t.Fatalf("expected mysql database, got %s", installer.opts.DatabaseEngine)
	}
	if installer.opts.HTTPPort != 8080 || installer.opts.HTTPSPort != 8443 {
		t.Fatalf("expected ports 8080/8443, got %d/%d", installer.opts.HTTPPort, installer.opts.HTTPSPort)
	}
	if installer.opts.SSLCertFile != "/tmp/cert.pem" || installer.opts.SSLKeyFile != "/tmp/key.pem" {
		t.Fatalf("expected ssl path overrides, got cert=%s key=%s", installer.opts.SSLCertFile, installer.opts.SSLKeyFile)
	}
}

func TestInstallCmdNoListenersFlag(t *testing.T) {
	installer := &captureRuntimeInstaller{}
	rootOpts := &rootOptions{}
	cmd := newInstallCmd(installer, rootOpts)
	cmd.SetArgs([]string{"--no-listeners"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}

	if installer.opts.ConfigureListeners == nil {
		t.Fatal("expected configureListeners override")
	}
	if *installer.opts.ConfigureListeners {
		t.Fatal("expected configureListeners=false")
	}
}
