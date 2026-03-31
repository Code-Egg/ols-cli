package platform

import (
	"context"
	"errors"
	"testing"

	"github.com/ols/ols-cli/internal/runner"
)

type fakeRunner struct {
	calls [][]string
	err   error
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) (runner.Result, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	if f.err != nil {
		return runner.Result{}, f.err
	}
	return runner.Result{}, nil
}

func TestPackageInstallerAPT(t *testing.T) {
	fr := &fakeRunner{}
	installer := NewPackageInstaller(fr, Info{PackageManager: PackageManagerAPT})

	if err := installer.Install(context.Background(), "openlitespeed", "lsphp82"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fr.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(fr.calls))
	}
	if fr.calls[0][0] != "apt-get" || fr.calls[0][1] != "update" {
		t.Fatalf("unexpected first call: %#v", fr.calls[0])
	}
	if fr.calls[1][0] != "apt-get" || fr.calls[1][1] != "install" {
		t.Fatalf("unexpected second call: %#v", fr.calls[1])
	}
}

func TestPackageInstallerCommandError(t *testing.T) {
	fr := &fakeRunner{err: errors.New("boom")}
	installer := NewPackageInstaller(fr, Info{PackageManager: PackageManagerYUM})

	if err := installer.Install(context.Background(), "openlitespeed"); err == nil {
		t.Fatal("expected error but got nil")
	}
}
