package cli

import "testing"

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
