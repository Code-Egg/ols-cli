package runner

import (
	"strings"
	"testing"
)

func TestRedactCommandArgsRedactsSensitiveValues(t *testing.T) {
	args := []string{
		"core",
		"install",
		"--admin_password=TopSecret123",
		"--dbpass=DbSecret456",
		"--token",
		"tok_abc123",
		"-pMyPassword",
		"--path=/var/www/example.com/html",
	}

	got := redactCommandArgs(args)
	joined := strings.Join(got, " ")

	if strings.Contains(joined, "TopSecret123") {
		t.Fatalf("expected admin password to be redacted, got: %v", got)
	}
	if strings.Contains(joined, "DbSecret456") {
		t.Fatalf("expected db password to be redacted, got: %v", got)
	}
	if strings.Contains(joined, "tok_abc123") {
		t.Fatalf("expected token to be redacted, got: %v", got)
	}
	if strings.Contains(joined, "MyPassword") {
		t.Fatalf("expected short -p password to be redacted, got: %v", got)
	}
	if !strings.Contains(joined, "--path=/var/www/example.com/html") {
		t.Fatalf("expected non-sensitive args to be preserved, got: %v", got)
	}
}

func TestRedactCommandArgsCaseInsensitiveKeys(t *testing.T) {
	args := []string{"--DB_PASSWORD=abc123", "--APIKEY", "xyz987"}
	got := redactCommandArgs(args)
	joined := strings.Join(got, " ")

	if strings.Contains(joined, "abc123") || strings.Contains(joined, "xyz987") {
		t.Fatalf("expected values to be redacted, got: %v", got)
	}
}
