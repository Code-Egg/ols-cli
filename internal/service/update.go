package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ols/ols-cli/internal/apperr"
)

const (
	defaultReleaseRepo = "Code-Egg/ols-cli"
	defaultBinaryName  = "ols"
)

type CLIUpdateOptions struct {
	DryRun bool
}

type cliUpdatePlan struct {
	Asset      string
	DownloadURL string
	BinaryPath string
}

func (s SiteService) UpdateCLI(_ context.Context, opts CLIUpdateOptions) error {
	s.console.Section("Update CLI")
	s.console.Info("Preparing self-update plan")

	plan, err := resolveCLIUpdatePlan(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}

	exePath, err := os.Executable()
	if err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to locate current executable", err)
	}
	plan.BinaryPath = filepath.Clean(exePath)

	s.console.Bullet("Current executable: " + plan.BinaryPath)
	s.console.Bullet("Release asset: " + plan.Asset)
	s.console.Bullet("Download URL: " + plan.DownloadURL)

	if opts.DryRun {
		s.console.Warn("Dry-run enabled: no system changes were made")
		s.console.Bullet("download latest release binary")
		s.console.Bullet("replace current executable atomically")
		s.console.Success("Dry-run plan generated")
		return nil
	}

	tmp, err := os.CreateTemp(filepath.Dir(plan.BinaryPath), ".ols-update-*")
	if err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to create temporary file for update", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return apperr.Wrap(apperr.CodeConfig, "failed to prepare temporary file for update", err)
	}
	defer func() { _ = os.Remove(tmpPath) }()

	if err := downloadToFile(plan.DownloadURL, tmpPath, 0o755); err != nil {
		return apperr.Wrap(apperr.CodeCommand, "failed to download latest release binary", err)
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to set permissions on downloaded binary", err)
	}
	if err := os.Rename(tmpPath, plan.BinaryPath); err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to replace ols binary", err)
	}

	s.console.Success("OLS CLI updated to latest release")
	s.console.Bullet("Binary replaced: " + plan.BinaryPath)
	return nil
}

func resolveCLIUpdatePlan(goos, goarch string) (cliUpdatePlan, error) {
	asset, err := releaseAssetForRuntime(goos, goarch)
	if err != nil {
		return cliUpdatePlan{}, err
	}
	return cliUpdatePlan{
		Asset:      asset,
		DownloadURL: latestReleaseAssetURL(defaultReleaseRepo, asset),
	}, nil
}

func releaseAssetForRuntime(goos, goarch string) (string, error) {
	osName := strings.TrimSpace(strings.ToLower(goos))
	archName := strings.TrimSpace(strings.ToLower(goarch))

	if osName != "linux" {
		return "", apperr.New(apperr.CodeValidation, fmt.Sprintf("unsupported OS for self-update: %q (supported: linux)", goos))
	}

	switch archName {
	case "x86_64":
		archName = "amd64"
	case "aarch64":
		archName = "arm64"
	}

	switch archName {
	case "amd64", "arm64":
		return fmt.Sprintf("%s-%s-%s", defaultBinaryName, osName, archName), nil
	default:
		return "", apperr.New(apperr.CodeValidation, fmt.Sprintf("unsupported architecture for self-update: %q (supported: amd64, arm64)", goarch))
	}
}

func latestReleaseAssetURL(repo, asset string) string {
	repoName := strings.TrimSpace(repo)
	if repoName == "" {
		repoName = defaultReleaseRepo
	}
	return fmt.Sprintf("https://github.com/%s/releases/latest/download/%s", repoName, strings.TrimSpace(asset))
}
