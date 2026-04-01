package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ols/ols-cli/internal/apperr"
)

type listenerDirective struct {
	key   string
	value string
}

func (s SiteService) configureRuntimeListeners(httpPort, httpsPort int, sslCertFile, sslKeyFile string) error {
	serverConfigPath := filepath.Join(s.lswsRoot, "conf", "httpd_config.conf")
	b, err := os.ReadFile(serverConfigPath)
	if err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to read OpenLiteSpeed server config", err)
	}
	cfg := string(b)

	updated, changedHTTP, err := upsertListenerDirectives(
		cfg,
		"Default",
		[]listenerDirective{{key: "address", value: fmt.Sprintf("*:%d", httpPort)}, {key: "secure", value: "0"}},
		[]string{"keyFile", "certFile"},
	)
	if err != nil {
		return err
	}

	updated, changedHTTPS, err := upsertListenerDirectives(
		updated,
		"SSL",
		[]listenerDirective{
			{key: "address", value: fmt.Sprintf("*:%d", httpsPort)},
			{key: "secure", value: "1"},
			{key: "keyFile", value: sslKeyFile},
			{key: "certFile", value: sslCertFile},
		},
		nil,
	)
	if err != nil {
		return err
	}

	if !changedHTTP && !changedHTTPS {
		s.console.Bullet("OpenLiteSpeed listeners already match requested defaults")
		return nil
	}

	if err := os.WriteFile(serverConfigPath, []byte(updated), 0o644); err != nil {
		return apperr.Wrap(apperr.CodeConfig, "failed to update OpenLiteSpeed server config", err)
	}

	s.console.Bullet("Updated server config: " + serverConfigPath)
	return nil
}

func upsertListenerDirectives(cfg, listenerName string, directives []listenerDirective, removeKeys []string) (string, bool, error) {
	lines := strings.Split(cfg, "\n")
	start, end := findListenerBlockByName(lines, listenerName)
	if start < 0 || end < 0 {
		block := buildListenerBlock(listenerName, directives)
		trimmed := strings.TrimRight(cfg, "\n")
		if trimmed == "" {
			return block + "\n", true, nil
		}
		return trimmed + "\n\n" + block + "\n", true, nil
	}

	if end-start < 1 {
		return "", false, apperr.New(apperr.CodeConfig, fmt.Sprintf("invalid listener block for %s", listenerName))
	}

	block := append([]string{}, lines[start:end+1]...)
	body := append([]string{}, block[1:len(block)-1]...)
	changed := false

	for _, key := range removeKeys {
		var keyChanged bool
		body, keyChanged = removeDirective(body, key)
		changed = changed || keyChanged
	}

	for _, d := range directives {
		if strings.TrimSpace(d.key) == "" {
			continue
		}
		var directiveChanged bool
		body, directiveChanged = upsertDirective(body, d.key, d.value)
		changed = changed || directiveChanged
	}

	if !changed {
		return cfg, false, nil
	}

	newBlock := []string{block[0]}
	newBlock = append(newBlock, body...)
	newBlock = append(newBlock, block[len(block)-1])

	updatedLines := append([]string{}, lines[:start]...)
	updatedLines = append(updatedLines, newBlock...)
	updatedLines = append(updatedLines, lines[end+1:]...)
	return strings.Join(updatedLines, "\n"), true, nil
}

func findListenerBlockByName(lines []string, target string) (int, int) {
	start := -1
	depth := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if start == -1 {
			name, ok := parseListenerName(trimmed)
			if !ok || !strings.EqualFold(name, target) {
				continue
			}
			start = i
			depth = strings.Count(line, "{") - strings.Count(line, "}")
			if depth <= 0 {
				depth = 1
			}
			continue
		}

		depth += strings.Count(line, "{")
		depth -= strings.Count(line, "}")
		if depth == 0 {
			return start, i
		}
	}
	return -1, -1
}

func parseListenerName(trimmedLine string) (string, bool) {
	if !strings.HasPrefix(trimmedLine, "listener ") || !strings.Contains(trimmedLine, "{") {
		return "", false
	}
	header := strings.TrimSpace(strings.SplitN(trimmedLine, "{", 2)[0])
	fields := strings.Fields(header)
	if len(fields) < 2 {
		return "", false
	}
	return fields[1], true
}

func detectDirectiveIndent(lines []string) string {
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) == 0 {
			continue
		}
		idx := strings.Index(line, fields[0])
		if idx > 0 {
			return line[:idx]
		}
	}
	return "  "
}

func formatDirectiveLine(indent, key, value string) string {
	return fmt.Sprintf("%s%-24s %s", indent, key, value)
}

func upsertDirective(lines []string, key, value string) ([]string, bool) {
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) == 0 || fields[0] != key {
			continue
		}
		indent := ""
		idx := strings.Index(line, fields[0])
		if idx > 0 {
			indent = line[:idx]
		}
		replacement := formatDirectiveLine(indent, key, value)
		if line == replacement {
			return lines, false
		}
		lines[i] = replacement
		return lines, true
	}

	indent := detectDirectiveIndent(lines)
	lines = append(lines, formatDirectiveLine(indent, key, value))
	return lines, true
}

func removeDirective(lines []string, key string) ([]string, bool) {
	res := make([]string, 0, len(lines))
	changed := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			fields := strings.Fields(trimmed)
			if len(fields) > 0 && fields[0] == key {
				changed = true
				continue
			}
		}
		res = append(res, line)
	}
	return res, changed
}

func buildListenerBlock(name string, directives []listenerDirective) string {
	lines := []string{fmt.Sprintf("listener %s {", name)}
	for _, d := range directives {
		if strings.TrimSpace(d.key) == "" {
			continue
		}
		lines = append(lines, formatDirectiveLine("  ", d.key, d.value))
	}
	lines = append(lines, "}")
	return strings.Join(lines, "\n")
}
