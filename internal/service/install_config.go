package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ols/ols-cli/internal/apperr"
	"github.com/ols/ols-cli/internal/platform"
)

const defaultInstallConfigPath = "/etc/ols-cli/install.json"

type RuntimeInstallConfig struct {
	PHPVersion         string `json:"php_version"`
	DatabaseEngine     string `json:"database"`
	ConfigureListeners *bool  `json:"configure_listeners,omitempty"`
	HTTPPort           int    `json:"http_port"`
	HTTPSPort          int    `json:"https_port"`
	SSLCertFile        string `json:"ssl_cert_file"`
	SSLKeyFile         string `json:"ssl_key_file"`
}

type resolvedInstallPlan struct {
	ConfigPath         string
	PHPVersion         string
	DatabaseEngine     string
	DatabasePackage    string
	ConfigureListeners bool
	HTTPPort           int
	HTTPSPort          int
	SSLCertFile        string
	SSLKeyFile         string
}

func defaultRuntimeInstallConfig(lswsRoot string) RuntimeInstallConfig {
	enabled := true
	return RuntimeInstallConfig{
		PHPVersion:         "85",
		DatabaseEngine:     "mariadb",
		ConfigureListeners: &enabled,
		HTTPPort:           80,
		HTTPSPort:          443,
		SSLCertFile:        filepath.Join(lswsRoot, "admin", "conf", "webadmin.crt"),
		SSLKeyFile:         filepath.Join(lswsRoot, "admin", "conf", "webadmin.key"),
	}
}

func loadRuntimeInstallConfig(path, lswsRoot string) (RuntimeInstallConfig, string, error) {
	cfg := defaultRuntimeInstallConfig(lswsRoot)
	resolvedPath := strings.TrimSpace(path)
	if resolvedPath == "" {
		resolvedPath = defaultInstallConfigPath
	}

	b, err := os.ReadFile(resolvedPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, resolvedPath, nil
		}
		return RuntimeInstallConfig{}, resolvedPath, apperr.Wrap(apperr.CodeConfig, "failed to read install config file", err)
	}

	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()

	var fileCfg RuntimeInstallConfig
	if err := dec.Decode(&fileCfg); err != nil {
		return RuntimeInstallConfig{}, resolvedPath, apperr.Wrap(apperr.CodeConfig, "failed to parse install config file", err)
	}

	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return RuntimeInstallConfig{}, resolvedPath, apperr.New(apperr.CodeConfig, "install config file contains extra trailing JSON values")
	}

	cfg = mergeRuntimeInstallConfig(cfg, fileCfg)
	return cfg, resolvedPath, nil
}

func mergeRuntimeInstallConfig(base, override RuntimeInstallConfig) RuntimeInstallConfig {
	if v := strings.TrimSpace(override.PHPVersion); v != "" {
		base.PHPVersion = v
	}
	if v := strings.TrimSpace(override.DatabaseEngine); v != "" {
		base.DatabaseEngine = v
	}
	if override.ConfigureListeners != nil {
		v := *override.ConfigureListeners
		base.ConfigureListeners = &v
	}
	if override.HTTPPort > 0 {
		base.HTTPPort = override.HTTPPort
	}
	if override.HTTPSPort > 0 {
		base.HTTPSPort = override.HTTPSPort
	}
	if v := strings.TrimSpace(override.SSLCertFile); v != "" {
		base.SSLCertFile = v
	}
	if v := strings.TrimSpace(override.SSLKeyFile); v != "" {
		base.SSLKeyFile = v
	}
	return base
}

func resolveInstallPlan(opts InstallOptions, info platform.Info, lswsRoot string) (resolvedInstallPlan, error) {
	cfg, cfgPath, err := loadRuntimeInstallConfig(opts.ConfigPath, lswsRoot)
	if err != nil {
		return resolvedInstallPlan{}, err
	}

	phpInput := cfg.PHPVersion
	if v := strings.TrimSpace(opts.PHPVersion); v != "" {
		phpInput = v
	}
	phpVersion, err := NormalizePHPVersion(phpInput)
	if err != nil {
		return resolvedInstallPlan{}, err
	}

	dbInput := cfg.DatabaseEngine
	if v := strings.TrimSpace(opts.DatabaseEngine); v != "" {
		dbInput = v
	}
	dbEngine, err := normalizeDatabaseEngine(dbInput)
	if err != nil {
		return resolvedInstallPlan{}, err
	}
	dbPackage, err := databasePackageFor(dbEngine, info.PackageManager)
	if err != nil {
		return resolvedInstallPlan{}, err
	}

	configureListeners := true
	if cfg.ConfigureListeners != nil {
		configureListeners = *cfg.ConfigureListeners
	}
	if opts.ConfigureListeners != nil {
		configureListeners = *opts.ConfigureListeners
	}

	httpPort := cfg.HTTPPort
	if opts.HTTPPort > 0 {
		httpPort = opts.HTTPPort
	}
	httpsPort := cfg.HTTPSPort
	if opts.HTTPSPort > 0 {
		httpsPort = opts.HTTPSPort
	}

	sslCertFile := strings.TrimSpace(cfg.SSLCertFile)
	if v := strings.TrimSpace(opts.SSLCertFile); v != "" {
		sslCertFile = v
	}
	sslKeyFile := strings.TrimSpace(cfg.SSLKeyFile)
	if v := strings.TrimSpace(opts.SSLKeyFile); v != "" {
		sslKeyFile = v
	}

	if err := validatePort(httpPort, "http_port"); err != nil {
		return resolvedInstallPlan{}, err
	}
	if err := validatePort(httpsPort, "https_port"); err != nil {
		return resolvedInstallPlan{}, err
	}
	if configureListeners {
		if sslCertFile == "" {
			return resolvedInstallPlan{}, apperr.New(apperr.CodeValidation, "ssl_cert_file is required when listener configuration is enabled")
		}
		if sslKeyFile == "" {
			return resolvedInstallPlan{}, apperr.New(apperr.CodeValidation, "ssl_key_file is required when listener configuration is enabled")
		}
		if err := validatePathLikeValue(sslCertFile, "ssl_cert_file"); err != nil {
			return resolvedInstallPlan{}, err
		}
		if err := validatePathLikeValue(sslKeyFile, "ssl_key_file"); err != nil {
			return resolvedInstallPlan{}, err
		}
	}

	return resolvedInstallPlan{
		ConfigPath:         cfgPath,
		PHPVersion:         phpVersion,
		DatabaseEngine:     dbEngine,
		DatabasePackage:    dbPackage,
		ConfigureListeners: configureListeners,
		HTTPPort:           httpPort,
		HTTPSPort:          httpsPort,
		SSLCertFile:        sslCertFile,
		SSLKeyFile:         sslKeyFile,
	}, nil
}

func normalizeDatabaseEngine(in string) (string, error) {
	candidate := strings.TrimSpace(strings.ToLower(in))
	if candidate == "" {
		candidate = "mariadb"
	}

	switch candidate {
	case "mariadb", "mysql", "none":
		return candidate, nil
	default:
		return "", apperr.New(apperr.CodeValidation, fmt.Sprintf("unsupported database engine: %q (allowed: mariadb,mysql,none)", in))
	}
}

func databasePackageFor(engine string, pm platform.PackageManager) (string, error) {
	switch engine {
	case "none":
		return "", nil
	case "mariadb":
		return "mariadb-server", nil
	case "mysql":
		switch pm {
		case platform.PackageManagerAPT:
			return "default-mysql-server", nil
		case platform.PackageManagerYUM, platform.PackageManagerDNF:
			return "mysql-server", nil
		default:
			return "", apperr.New(apperr.CodePlatform, fmt.Sprintf("unsupported package manager: %s", pm))
		}
	default:
		return "", apperr.New(apperr.CodeValidation, fmt.Sprintf("unsupported database engine: %q", engine))
	}
}

func validatePort(port int, field string) error {
	if port < 1 || port > 65535 {
		return apperr.New(apperr.CodeValidation, fmt.Sprintf("%s must be between 1 and 65535", field))
	}
	return nil
}

func validatePathLikeValue(v, field string) error {
	candidate := strings.TrimSpace(v)
	if candidate == "" {
		return apperr.New(apperr.CodeValidation, fmt.Sprintf("%s is required", field))
	}
	if strings.ContainsAny(candidate, "\r\n\x00") {
		return apperr.New(apperr.CodeValidation, fmt.Sprintf("%s contains unsafe control characters", field))
	}
	for _, r := range candidate {
		if r < 0x20 {
			return apperr.New(apperr.CodeValidation, fmt.Sprintf("%s contains unsafe control characters", field))
		}
	}
	return nil
}
