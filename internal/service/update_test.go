package service

import "testing"

func TestReleaseAssetForRuntime(t *testing.T) {
	asset, err := releaseAssetForRuntime("linux", "amd64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if asset != "ols-linux-amd64" {
		t.Fatalf("unexpected asset: %s", asset)
	}

	asset, err = releaseAssetForRuntime("linux", "arm64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if asset != "ols-linux-arm64" {
		t.Fatalf("unexpected asset: %s", asset)
	}
}

func TestReleaseAssetForRuntimeRejectsUnsupported(t *testing.T) {
	if _, err := releaseAssetForRuntime("darwin", "arm64"); err == nil {
		t.Fatal("expected error for unsupported os")
	}
	if _, err := releaseAssetForRuntime("linux", "386"); err == nil {
		t.Fatal("expected error for unsupported arch")
	}
}

func TestLatestReleaseAssetURL(t *testing.T) {
	got := latestReleaseAssetURL("Code-Egg/ols-cli", "ols-linux-amd64")
	want := "https://github.com/Code-Egg/ols-cli/releases/latest/download/ols-linux-amd64"
	if got != want {
		t.Fatalf("unexpected release url: got=%s want=%s", got, want)
	}
}
