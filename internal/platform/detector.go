package platform

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/eric7/ols-cli/internal/apperr"
)

type Family string

type PackageManager string

const (
	FamilyDebian Family = "debian"
	FamilyRHEL   Family = "rhel"
	FamilyOther  Family = "other"
)

const (
	PackageManagerAPT PackageManager = "apt"
	PackageManagerYUM PackageManager = "yum"
	PackageManagerDNF PackageManager = "dnf"
)

type Info struct {
	ID             string
	IDLike         string
	VersionID      string
	Family         Family
	PackageManager PackageManager
}

type Detector interface {
	Detect(ctx context.Context) (Info, error)
}

type OSReleaseDetector struct {
	Path string
}

func NewOSReleaseDetector(path string) OSReleaseDetector {
	if strings.TrimSpace(path) == "" {
		path = "/etc/os-release"
	}
	return OSReleaseDetector{Path: path}
}

func (d OSReleaseDetector) Detect(_ context.Context) (Info, error) {
	content, err := os.ReadFile(d.Path)
	if err != nil {
		return Info{}, apperr.Wrap(apperr.CodePlatform, "failed to read /etc/os-release", err)
	}

	kv := ParseOSRelease(string(content))
	id := strings.ToLower(strings.TrimSpace(kv["ID"]))
	idLike := strings.ToLower(strings.TrimSpace(kv["ID_LIKE"]))
	versionID := strings.TrimSpace(kv["VERSION_ID"])
	family := InferFamily(id, idLike)
	pm := InferPackageManager(id, family, versionID)

	if family == FamilyOther {
		return Info{}, apperr.New(apperr.CodePlatform, "unsupported Linux distribution")
	}

	return Info{
		ID:             id,
		IDLike:         idLike,
		VersionID:      versionID,
		Family:         family,
		PackageManager: pm,
	}, nil
}

func ParseOSRelease(content string) map[string]string {
	result := make(map[string]string)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, "\"")
		result[key] = val
	}
	return result
}

func InferFamily(id, idLike string) Family {
	id = strings.ToLower(id)
	idLike = strings.ToLower(idLike)
	if isDebianLike(id, idLike) {
		return FamilyDebian
	}
	if isRHELLike(id, idLike) {
		return FamilyRHEL
	}
	return FamilyOther
}

func InferPackageManager(id string, family Family, versionID string) PackageManager {
	if family == FamilyDebian {
		return PackageManagerAPT
	}
	if family != FamilyRHEL {
		return ""
	}

	major := parseMajorVersion(versionID)
	if strings.Contains(id, "centos") && major > 0 && major < 8 {
		return PackageManagerYUM
	}
	if strings.Contains(id, "amzn") {
		if major == 2 {
			return PackageManagerYUM
		}
		return PackageManagerDNF
	}
	if major > 0 && major < 8 {
		return PackageManagerYUM
	}
	return PackageManagerDNF
}

func parseMajorVersion(versionID string) int {
	clean := strings.TrimSpace(strings.Trim(versionID, "\""))
	if clean == "" {
		return 0
	}
	segment := clean
	if strings.Contains(clean, ".") {
		segment = strings.SplitN(clean, ".", 2)[0]
	}
	major, err := strconv.Atoi(segment)
	if err != nil {
		return 0
	}
	return major
}

func isDebianLike(id, idLike string) bool {
	values := strings.Fields(strings.ReplaceAll(idLike, ",", " "))
	values = append(values, id)
	for _, v := range values {
		if v == "debian" || v == "ubuntu" {
			return true
		}
	}
	return false
}

func isRHELLike(id, idLike string) bool {
	values := strings.Fields(strings.ReplaceAll(idLike, ",", " "))
	values = append(values, id)
	for _, v := range values {
		switch v {
		case "rhel", "fedora", "centos", "rocky", "almalinux", "amzn":
			return true
		}
	}
	return false
}

func (i Info) Summary() string {
	return fmt.Sprintf("id=%s family=%s packageManager=%s version=%s", i.ID, i.Family, i.PackageManager, i.VersionID)
}
