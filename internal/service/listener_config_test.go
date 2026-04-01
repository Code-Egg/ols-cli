package service

import (
	"strings"
	"testing"
)

func TestUpsertListenerDirectivesCreatesListener(t *testing.T) {
	cfg := "virtualhost example.com {\n  vhRoot                  /var/www/example.com/\n}\n"

	updated, changed, err := upsertListenerDirectives(
		cfg,
		"Default",
		[]listenerDirective{{key: "address", value: "*:80"}, {key: "secure", value: "0"}},
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	if !strings.Contains(updated, "listener Default {") {
		t.Fatalf("expected listener block, got: %s", updated)
	}
	if !strings.Contains(updated, "address                  *:80") {
		t.Fatalf("expected address directive, got: %s", updated)
	}
}

func TestUpsertListenerDirectivesUpdatesExisting(t *testing.T) {
	cfg := "listener Default {\n  address                 *:8088\n  secure                  0\n}\n"

	updated, changed, err := upsertListenerDirectives(
		cfg,
		"Default",
		[]listenerDirective{{key: "address", value: "*:80"}, {key: "secure", value: "0"}},
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	if !strings.Contains(updated, "address                 *:80") {
		t.Fatalf("expected updated address, got: %s", updated)
	}
	if strings.Contains(updated, "*:8088") {
		t.Fatalf("expected old address removed, got: %s", updated)
	}
}

func TestUpsertListenerDirectivesNoChange(t *testing.T) {
	cfg := "listener Default {\n  address                 *:80\n  secure                  0\n}\n"

	updated, changed, err := upsertListenerDirectives(
		cfg,
		"Default",
		[]listenerDirective{{key: "address", value: "*:80"}, {key: "secure", value: "0"}},
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed {
		t.Fatal("expected changed=false")
	}
	if updated != cfg {
		t.Fatalf("expected config unchanged")
	}
}

func TestUpsertListenerDirectivesRemoveKeys(t *testing.T) {
	cfg := "listener Default {\n  address                 *:80\n  secure                  1\n  keyFile                 /tmp/key\n  certFile                /tmp/cert\n}\n"

	updated, changed, err := upsertListenerDirectives(
		cfg,
		"Default",
		[]listenerDirective{{key: "secure", value: "0"}},
		[]string{"keyFile", "certFile"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	if strings.Contains(updated, "keyFile") || strings.Contains(updated, "certFile") {
		t.Fatalf("expected key/cert directives removed, got: %s", updated)
	}
}

func TestEnsureDomainMapsCopiedToSSL(t *testing.T) {
	cfg := "listener Default {\n  address                 *:80\n  secure                  0\n  map                     example.com example.com\n}\n\nlistener SSL {\n  address                 *:443\n  secure                  1\n  keyFile                 /tmp/key\n  certFile                /tmp/cert\n}\n"

	updated, changed := ensureDomainMapsCopied(cfg, "Default", "SSL")
	if !changed {
		t.Fatal("expected changed=true")
	}
	if !strings.Contains(updated, "listener SSL {") {
		t.Fatalf("expected SSL listener present, got: %s", updated)
	}
	if !strings.Contains(updated, "map                     example.com example.com") {
		t.Fatalf("expected map copied to SSL listener, got: %s", updated)
	}
}

func TestEnsureDomainMapsCopiedNoChangeWhenAlreadyMapped(t *testing.T) {
	cfg := "listener Default {\n  address                 *:80\n  secure                  0\n  map                     example.com example.com\n}\n\nlistener SSL {\n  address                 *:443\n  secure                  1\n  map                     example.com example.com\n}\n"

	updated, changed := ensureDomainMapsCopied(cfg, "Default", "SSL")
	if changed {
		t.Fatal("expected changed=false")
	}
	if updated != cfg {
		t.Fatalf("expected config unchanged")
	}
}
