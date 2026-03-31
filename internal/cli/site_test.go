package cli

import "testing"

func TestPHPFlagsSelected(t *testing.T) {
	flags := phpFlags{v82: true}
	got, err := flags.selected("82")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "82" {
		t.Fatalf("expected 82, got %s", got)
	}
}

func TestPHPFlagsSelectedMultiple(t *testing.T) {
	flags := phpFlags{v82: true, v83: true}
	if _, err := flags.selected("82"); err == nil {
		t.Fatal("expected validation error for multiple php flags")
	}
}

func TestPHPFlagsSelectedDefault(t *testing.T) {
	flags := phpFlags{}
	got, err := flags.selected("82")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "82" {
		t.Fatalf("expected default 82, got %s", got)
	}
}
