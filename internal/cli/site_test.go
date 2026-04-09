package cli

import (
	"context"
	"testing"

	"github.com/ols/ols-cli/internal/service"
	"github.com/spf13/pflag"
)

type noopSiteManager struct{}

func (noopSiteManager) CreateSite(context.Context, service.CreateSiteOptions) error {
	return nil
}

func (noopSiteManager) UpdateSitePHP(context.Context, service.UpdateSiteOptions) error {
	return nil
}

func (noopSiteManager) SiteInfo(context.Context, service.SiteInfoOptions) error {
	return nil
}

func (noopSiteManager) ShowSiteConfig(context.Context, service.ShowSiteConfigOptions) error {
	return nil
}

func (noopSiteManager) ListSites(context.Context, service.ListSitesOptions) error {
	return nil
}

func (noopSiteManager) DeleteSite(context.Context, service.DeleteSiteOptions) error {
	return nil
}

func TestPHPFlagsSelected(t *testing.T) {
	flags := phpFlags{v85: true}
	got, err := flags.selected("85")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "85" {
		t.Fatalf("expected 85, got %s", got)
	}
}

func TestPHPFlagsSelectedMultiple(t *testing.T) {
	flags := phpFlags{v85: true, v83: true}
	if _, err := flags.selected("85"); err == nil {
		t.Fatal("expected validation error for multiple php flags")
	}
}

func TestPHPFlagsSelectedDefault(t *testing.T) {
	flags := phpFlags{}
	got, err := flags.selected("85")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "85" {
		t.Fatalf("expected default 85, got %s", got)
	}
}

func TestToggleFlagsSelectedEnable(t *testing.T) {
	flags := toggleFlags{enable: true}
	got, err := flags.selected("owasp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || !*got {
		t.Fatal("expected enabled toggle")
	}
}

func TestToggleFlagsSelectedDisable(t *testing.T) {
	flags := toggleFlags{disable: true}
	got, err := flags.selected("recaptcha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || *got {
		t.Fatal("expected disabled toggle")
	}
}

func TestToggleFlagsSelectedConflict(t *testing.T) {
	flags := toggleFlags{enable: true, disable: true}
	if _, err := flags.selected("owasp"); err == nil {
		t.Fatal("expected validation error for conflicting toggle flags")
	}
}

func TestSiteCreateFlagOrder(t *testing.T) {
	cmd := newSiteCreateCmd(noopSiteManager{}, &rootOptions{})
	if cmd.Flags().SortFlags {
		t.Fatal("expected SortFlags=false for create command")
	}
	cmd.InitDefaultHelpFlag()

	var got []string
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		got = append(got, f.Name)
	})

	wantPrefix := []string{
		"wp",
		"le",
		"php81",
		"php82",
		"php83",
		"php84",
		"php85",
		"enable-owasp",
		"enable-recaptcha",
		"disable-owasp",
		"disable-recaptcha",
		"hsts",
	}
	if len(got) < len(wantPrefix) {
		t.Fatalf("expected at least %d flags, got %d: %#v", len(wantPrefix), len(got), got)
	}
	for i := range wantPrefix {
		if got[i] != wantPrefix[i] {
			t.Fatalf("unexpected create flag order at index %d: got %q want %q (full=%#v)", i, got[i], wantPrefix[i], got)
		}
	}
}

func TestSiteUpdateFlagOrder(t *testing.T) {
	cmd := newSiteUpdateCmd(noopSiteManager{}, &rootOptions{})
	if cmd.Flags().SortFlags {
		t.Fatal("expected SortFlags=false for update command")
	}
	cmd.InitDefaultHelpFlag()

	var got []string
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		got = append(got, f.Name)
	})

	wantPrefix := []string{
		"wp",
		"php81",
		"php82",
		"php83",
		"php84",
		"php85",
		"enable-owasp",
		"enable-recaptcha",
		"disable-owasp",
		"disable-recaptcha",
		"hsts",
	}
	if len(got) < len(wantPrefix) {
		t.Fatalf("expected at least %d flags, got %d: %#v", len(wantPrefix), len(got), got)
	}
	for i := range wantPrefix {
		if got[i] != wantPrefix[i] {
			t.Fatalf("unexpected update flag order at index %d: got %q want %q (full=%#v)", i, got[i], wantPrefix[i], got)
		}
	}
}
