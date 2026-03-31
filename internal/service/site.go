package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/ols/ols-cli/internal/apperr"
	"github.com/ols/ols-cli/internal/platform"
	"github.com/ols/ols-cli/internal/runner"
	"github.com/ols/ols-cli/internal/ui"
)

var domainPattern = regexp.MustCompile(`(?i)^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)

// SiteService contains site lifecycle workflows.
type SiteService struct {
	detector platform.Detector
	runner   runner.Runner
	console  ui.Console
}

func NewSiteService(detector platform.Detector, run runner.Runner, console ui.Console) SiteService {
	return SiteService{detector: detector, runner: run, console: console}
}

type CreateSiteOptions struct {
	Domain        string
	WithWordPress bool
	WithLE        bool
	PHPVersion    string
	DryRun        bool
}

type UpdateSiteOptions struct {
	Domain     string
	PHPVersion string
	DryRun     bool
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

	s.console.Section("Create site")
	s.console.Bullet("Domain: " + opts.Domain)
	s.console.Bullet("PHP: lsphp" + phpVersion)
	s.console.Bullet("Platform: " + info.Summary())
	if opts.WithWordPress {
		s.console.Bullet("WordPress: enabled")
	}
	if opts.WithLE {
		s.console.Bullet("Let's Encrypt: enabled")
	}

	packages := packagesForCreate(info, phpVersion, opts.WithWordPress, opts.WithLE)
	if opts.DryRun {
		s.console.Warn("Dry-run enabled: no system changes were made")
		s.console.Info("Planned package install:")
		for _, pkg := range packages {
			s.console.Bullet(pkg)
		}
		s.console.Success("Dry-run plan generated")
		return nil
	}

	installer := platform.NewPackageInstaller(s.runner, info)
	if err := installer.Install(ctx, packages...); err != nil {
		return err
	}

	s.console.Success("Base packages installed")
	s.console.Warn("Virtual host, WordPress bootstrap, and SSL apply are scaffolded next steps")
	return nil
}

func (s SiteService) UpdateSitePHP(ctx context.Context, opts UpdateSiteOptions) error {
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

	s.console.Section("Update site PHP")
	s.console.Bullet("Domain: " + opts.Domain)
	s.console.Bullet("Target: lsphp" + phpVersion)
	s.console.Bullet("Platform: " + info.Summary())

	packages := packagesForPHPUpdate(phpVersion)
	if opts.DryRun {
		s.console.Warn("Dry-run enabled: no system changes were made")
		s.console.Info("Planned package install:")
		for _, pkg := range packages {
			s.console.Bullet(pkg)
		}
		s.console.Success("Dry-run plan generated")
		return nil
	}

	installer := platform.NewPackageInstaller(s.runner, info)
	if err := installer.Install(ctx, packages...); err != nil {
		return err
	}

	s.console.Success("Requested PHP package installed")
	s.console.Warn("OpenLiteSpeed vhost PHP handler switch is scaffolded next step")
	return nil
}

func NormalizePHPVersion(in string) (string, error) {
	candidate := strings.TrimSpace(strings.ToLower(in))
	candidate = strings.TrimPrefix(candidate, "lsphp")
	candidate = strings.ReplaceAll(candidate, ".", "")

	supported := map[string]struct{}{
		"74": {},
		"80": {},
		"81": {},
		"82": {},
		"83": {},
		"84": {},
	}

	if _, ok := supported[candidate]; !ok {
		return "", apperr.New(
			apperr.CodeValidation,
			fmt.Sprintf("unsupported PHP version: %q (allowed: 74,80,81,82,83,84)", in),
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

func packagesForCreate(info platform.Info, phpVersion string, withWP, withLE bool) []string {
	pkgs := []string{"openlitespeed", "lsphp" + phpVersion, "lsphp" + phpVersion + "-mysql"}
	if withWP {
		dbClient := "mariadb-client"
		if info.Family == platform.FamilyRHEL {
			dbClient = "mariadb"
		}
		pkgs = append(pkgs, "curl", "unzip", dbClient)
	}
	if withLE {
		pkgs = append(pkgs, "certbot")
	}
	return pkgs
}

func packagesForPHPUpdate(phpVersion string) []string {
	return []string{"lsphp" + phpVersion, "lsphp" + phpVersion + "-mysql"}
}
