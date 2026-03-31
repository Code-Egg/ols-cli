# ols-cli

A practical CLI for managing the OpenLiteSpeed + WordPress lifecycle on Linux servers.

It is currently focused on clear command behavior, predictable output, and safe automation patterns.

## Current scope

- Linux distro targeting:
  - Ubuntu / Debian
  - CentOS-family (CentOS, Rocky, AlmaLinux)
- Commands available:
  - `ols site create <domain> --wp [--le] [--php74|--php80|--php81|--php82|--php83|--php84]`
  - `ols site update <domain> --php74|--php80|--php81|--php82|--php83|--php84`
- Structured errors with stable machine-readable codes
- Styled terminal output for clearer operations
- Unit tests and CI scaffold

## Why Go

- Single compiled binary for deployment consistency
- Strong type safety for privileged automation code
- Built-in test tooling with straightforward CI integration

If release binaries are published, end users do not need to compile from source.

## Installation channels

### 1) Bash bootstrap installer

```bash
bash ols
```

The `ols` script calls `scripts/install.sh`, downloads release artifacts, verifies SHA-256 checksums, and installs `ols` to `/usr/local/bin/ols`.

### 2) npm package scaffold

Directory: `packaging/npm`

- Package name: `@ols/cli`
- `postinstall.js` downloads and verifies Linux binary from GitHub Releases
- `bin/ols` launches the installed binary

### 3) apt packaging scaffold

Directory: `packaging/apt`

Contains Debian packaging metadata (`debian/control`, `debian/rules`, etc.) for future `apt install ols` publishing.

## One-time server bootstrap (recommended)

Before first site provisioning, add the LiteSpeed package repository once:

```bash
wget -O - https://repo.litespeed.sh | sudo bash
```

Then install `ols` and run site commands.

## Usage

### Create a site with WordPress + Let's Encrypt

```bash
ols site create example.com --wp --le --php82
```

Current implementation note: this command currently installs required packages and validates workflow wiring. It does **not** yet create the OpenLiteSpeed virtual host, document root, or WordPress files.

### Create a site with defaults (WordPress + PHP 8.2)

```bash
ols site create example.com --wp
```

### Update a site to a target PHP version

```bash
ols site update example.com --php82
```

### Preview operations without making changes

```bash
ols --dry-run site create example.com --wp --le --php82
```

## Development

### Build locally

```bash
go build -o ols ./cmd/ols
```

### Run tests

```bash
go test ./...
```

For verbose output:

```bash
go test -v ./...
```

## Testing on an Ubuntu server

Use these steps on your Ubuntu host (22.04/24.04 recommended):

1. Install Go and Git

```bash
sudo apt update
sudo apt install -y golang-go git build-essential
```

2. Clone and enter the project

```bash
git clone https://github.com/<your-org>/ols-cli.git
cd ols-cli
```

3. Sync modules and generate checksums

```bash
go mod tidy
```

This creates/updates `go.sum`, which is required for reproducible builds and CI.

4. Run tests

```bash
go test ./...
```

5. Build the CLI

```bash
go build -o ols ./cmd/ols
```

6. Smoke-test command parsing (safe)

```bash
./ols --help
./ols --dry-run site create example.com --wp --le --php82
```

## Common build issue: missing go.sum entries

If you see errors like “missing go.sum entry for module providing package ...”, run:

```bash
go mod tidy
```

Then commit both files:

- `go.mod`
- `go.sum`

This keeps CI and local builds in sync.

## CI

Workflow: `.github/workflows/ci.yml`

## Security notes

- Input validation for domain and PHP version flags
- Explicit command execution abstraction in `internal/runner`
- Fail-fast structured errors in `internal/apperr`
- Checksum verification in installer/downloader scaffolds

## Project status

This repository is a strong scaffold with tested core abstractions and command routing.

The following are planned next phases:

- OpenLiteSpeed virtual host file generation and activation
- WordPress provisioning (download, config, database bootstrap)
- Let's Encrypt issuance and SSL virtual host wiring
- Full Debian repository publishing pipeline
