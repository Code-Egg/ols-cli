# ols-cli

Security-focused OpenLiteSpeed lifecycle CLI for WordPress sites.

## Current scope

- Cross-distro Linux targeting:
  - Ubuntu / Debian
  - CentOS-family (CentOS, Rocky, AlmaLinux)
- Commands scaffolded:
  - `ols site create <domain> --wp [--le] [--php82]`
  - `ols site update <domain> --php82`
- Strong structured error model with machine-readable error codes.
- Styled console output for operational clarity.
- Automated tests and CI workflow scaffold.

## Why Go

- Compiled, static binary for predictable production deployment.
- Strong typing and standard test tooling (`go test ./...`).
- Better safety profile for privileged automation than ad-hoc shell-only logic.

Users do **not** need to compile locally when binaries are published in releases.

## Installation channels

### 1) Bash bootstrap installer

```bash
bash ols
```

`ols` is a thin bootstrap entry script that calls [`scripts/install.sh`](scripts/install.sh).
It downloads release artifacts, verifies SHA-256 checksums, and installs `ols` to `/usr/local/bin/ols`.

### 2) npm package scaffold

Directory: `packaging/npm`

- Package name: `@ols/cli`
- `postinstall.js` downloads and verifies Linux binary from GitHub Releases.
- `bin/ols` launches installed binary.

### 3) apt packaging scaffold

Directory: `packaging/apt`

Contains initial Debian packaging metadata scaffold (`debian/control`, `debian/rules`, etc.) for publishing `apt install ols`.

## Usage

### Create a site with WordPress + Let's Encrypt

```bash
ols site create example.com --wp --le --php82
```

### Create a site with defaults (PHP 8.2)

```bash
ols site create example.com --wp
```

### Update a site to PHP 8.2

```bash
ols site update example.com --php82
```

### Preview operations without changing server

```bash
ols --dry-run site create example.com --wp --le --php82
```

## Development

### Build

```bash
go build -o ols ./cmd/ols
```

### Test

```bash
go test ./...
```

### CI

GitHub Actions workflow: `.github/workflows/ci.yml`

## Security notes

- Input validation for domain and PHP version flags.
- Explicit command execution abstraction (`internal/runner`) for safer process control.
- Fail-fast errors with structured codes under `internal/apperr`.
- Checksum verification in installer/downloader scaffolds.

## Important status

This is a strong scaffold with tested core abstractions and command routing.
The following are intentionally marked as next implementation phases:

- OpenLiteSpeed vhost file generation and activation.
- WordPress provisioning (download/config/db bootstrap).
- Let's Encrypt issuance and SSL vhost wiring.
- Full Debian repository publishing pipeline.
