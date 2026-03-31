package platform

import "testing"

func TestParseOSRelease(t *testing.T) {
	content := `ID="ubuntu"
ID_LIKE=debian
VERSION_ID="22.04"
`
	parsed := ParseOSRelease(content)
	if parsed["ID"] != "ubuntu" {
		t.Fatalf("expected ID ubuntu, got %q", parsed["ID"])
	}
	if parsed["ID_LIKE"] != "debian" {
		t.Fatalf("expected ID_LIKE debian, got %q", parsed["ID_LIKE"])
	}
	if parsed["VERSION_ID"] != "22.04" {
		t.Fatalf("expected VERSION_ID 22.04, got %q", parsed["VERSION_ID"])
	}
}

func TestInferFamily(t *testing.T) {
	if got := InferFamily("ubuntu", "debian"); got != FamilyDebian {
		t.Fatalf("expected debian family, got %s", got)
	}
	if got := InferFamily("centos", "rhel fedora"); got != FamilyRHEL {
		t.Fatalf("expected rhel family, got %s", got)
	}
	if got := InferFamily("arch", ""); got != FamilyOther {
		t.Fatalf("expected other family, got %s", got)
	}
}

func TestInferPackageManager(t *testing.T) {
	if got := InferPackageManager("ubuntu", FamilyDebian, "22.04"); got != PackageManagerAPT {
		t.Fatalf("expected apt, got %s", got)
	}
	if got := InferPackageManager("centos", FamilyRHEL, "7"); got != PackageManagerYUM {
		t.Fatalf("expected yum for centos 7, got %s", got)
	}
	if got := InferPackageManager("centos", FamilyRHEL, "8"); got != PackageManagerDNF {
		t.Fatalf("expected dnf for centos 8, got %s", got)
	}
}
