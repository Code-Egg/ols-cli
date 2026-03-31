package platform

import (
	"context"
	"fmt"
	"strings"

	"github.com/eric7/ols-cli/internal/apperr"
	"github.com/eric7/ols-cli/internal/runner"
)

type PackageInstaller struct {
	runner runner.Runner
	info   Info
}

func NewPackageInstaller(run runner.Runner, info Info) PackageInstaller {
	return PackageInstaller{runner: run, info: info}
}

func (i PackageInstaller) Install(ctx context.Context, packages ...string) error {
	pkgs := dedupeAndSanitize(packages)
	if len(pkgs) == 0 {
		return apperr.New(apperr.CodeValidation, "no packages provided")
	}

	var err error
	switch i.info.PackageManager {
	case PackageManagerAPT:
		_, err = i.runner.Run(ctx, "apt-get", append([]string{"update"}, []string{}...)...)
		if err != nil {
			return apperr.Wrap(apperr.CodeCommand, "apt metadata refresh failed", err)
		}
		_, err = i.runner.Run(ctx, "apt-get", append([]string{"install", "-y"}, pkgs...)...)
	case PackageManagerYUM:
		_, err = i.runner.Run(ctx, "yum", append([]string{"install", "-y"}, pkgs...)...)
	case PackageManagerDNF:
		_, err = i.runner.Run(ctx, "dnf", append([]string{"install", "-y"}, pkgs...)...)
	default:
		return apperr.New(apperr.CodePlatform, fmt.Sprintf("unsupported package manager: %s", i.info.PackageManager))
	}

	if err != nil {
		return apperr.Wrap(apperr.CodeCommand, "package installation failed", err)
	}

	return nil
}

func dedupeAndSanitize(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	res := make([]string, 0, len(items))
	for _, item := range items {
		i := strings.TrimSpace(item)
		if i == "" {
			continue
		}
		if _, ok := seen[i]; ok {
			continue
		}
		seen[i] = struct{}{}
		res = append(res, i)
	}
	return res
}
