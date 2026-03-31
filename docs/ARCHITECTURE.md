# OLS CLI Architecture

This document defines the initial architecture for `ols`, a security-focused CLI for OpenLiteSpeed + WordPress lifecycle management.

## Goals

- Linux server support for Ubuntu, Debian, and CentOS-family distributions.
- Predictable, testable automation for:
  - `ols site create <domain> --wp [--le]`
  - `ols site update <domain> --phpXX`
- Strong error handling with stable error codes.
- Beautiful and readable console output.
- No code copy from WordOps/ols1clk; behavior-inspired design only.

## Language Choice

- Core CLI: Go
  - Single static binary deployment
  - Strong typing and robust test tooling
  - Good fit for privileged server automation
- Installer channels:
  - apt (Debian/Ubuntu package)
  - npm (binary bootstrap/downloader package)

## High-Level Layout

```text
cmd/ols/main.go               # binary entrypoint
internal/cli/                 # Cobra command graph
internal/platform/            # OS detection + package manager logic
internal/service/             # business workflows (site create/update)
internal/runner/              # command execution abstraction
internal/ui/                  # styled console output
internal/apperr/              # structured application errors
scripts/install.sh            # bash bootstrap installer
```

## Security Baseline

- Validate and sanitize all user inputs (domain, php switch flags).
- Explicit command execution through a small runner abstraction.
- Prefer non-shell invocation (`exec.CommandContext`) to avoid shell injection.
- Fail fast with actionable errors and no silent fallback.
- Dry-run support to preview privileged operations.

## Testing Strategy

- Unit tests for:
  - domain validation
n  - distro detection parser
  - php flag selection
  - package manager command generation
- Command behavior tests with fake runner (no real package installs).
- CI gate: `go test ./...` must pass before release artifacts.

## Delivery Strategy

- Release pipeline builds Linux binaries for `amd64` and `arm64`.
- `scripts/install.sh` downloads and verifies release binary.
- apt package and npm package act as distribution channels to install the same signed binary.
