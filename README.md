# OLS CLI

A practical CLI for managing the OpenLiteSpeed + WordPress lifecycle on Linux servers.

It is currently focused on clear command behavior, predictable output, and safe automation patterns.

## Current scope

- Linux distro targeting:
  - Ubuntu / Debian
  - CentOS-family (CentOS, Rocky, AlmaLinux)
- Commands available:
  - `ols install [--php81|--php82|--php83|--php84|--php85] [--database mariadb|mysql|none] [--config /path/install.json]`
  - `ols site create <domain> --wp [--le] [--php81|--php82|--php83|--php84|--php85]`
  - `ols site update <domain> [--wp] --php81|--php82|--php83|--php84|--php85`
- Structured errors with stable machine-readable codes
- Styled terminal output for clearer operations
- Unit tests and CI scaffold

## Why Go

- Single compiled binary for deployment consistency
- Strong type safety for privileged automation code
- Built-in test tooling with straightforward CI integration

If release binaries are published, end users do not need to compile from source.

## Installation channels

### 1) Direct binary download (recommended)

Pick the correct architecture (`amd64` or `arm64`) and download directly from GitHub Releases.

```bash
sudo wget -O /usr/local/bin/ols https://github.com/Code-Egg/ols-cli/releases/latest/download/ols-linux-amd64
sudo chmod +x /usr/local/bin/ols
ols --help
```

For ARM64 servers:

```bash
sudo wget -O /usr/local/bin/ols https://github.com/Code-Egg/ols-cli/releases/latest/download/ols-linux-arm64
sudo chmod +x /usr/local/bin/ols
ols --help
```

Optional checksum verification:

```bash
wget https://github.com/Code-Egg/ols-cli/releases/latest/download/checksums.txt
sha256sum /usr/local/bin/ols
```

If you need reproducible installs, pin a specific tag instead of `latest`.

If colors do not appear in your terminal, force them explicitly:

```bash
ols --color=always --help
```

You can also disable colors when needed:

```bash
ols --color=never --help
```

### 2) Bash bootstrap installer

```bash
bash ols
```

The `ols` script calls `scripts/install.sh`, downloads release artifacts, verifies SHA-256 checksums, and installs `ols` to `/usr/local/bin/ols`.

### 3) npm package scaffold

Directory: `packaging/npm`

- Package name: `@ols/cli`
- `postinstall.js` downloads and verifies Linux binary from GitHub Releases
- `bin/ols` launches the installed binary

### 4) apt packaging scaffold

Directory: `packaging/apt`

Contains Debian packaging metadata (`debian/control`, `debian/rules`, etc.) for future `apt install ols` publishing.

## One-time server bootstrap (recommended)

Use `ols install` before first site provisioning. It configures LiteSpeed repository access, installs OpenLiteSpeed + selected PHP runtime + selected database, and can configure listeners for HTTP/HTTPS.

```bash
sudo ols install --php85 --database mariadb
```

Then create sites with `ols site create ...`.

## Usage

### Install runtime once (required before provisioning)

```bash
sudo ols install --php85 --database mariadb
```

You can switch database engine:

```bash
sudo ols install --php83 --database mysql
sudo ols install --php85 --database none
```

Use config file defaults (recommended for first-run bootstrap):

```bash
sudo mkdir -p /etc/ols-cli
sudo cp docs/install.example.json /etc/ols-cli/install.json
sudo ols install
```

Override config values with flags when needed:

```bash
sudo ols install --config /etc/ols-cli/install.json --php85 --database mysql --http-port 80 --https-port 443
sudo ols install --no-listeners
```

### Create a site with WordPress + Let's Encrypt

```bash
sudo ols site create example.com --wp --le --php85
```

This command now creates:
- Virtual host config directory under `/usr/local/lsws/conf/vhosts/<domain>/`
- `vhconf.conf` and `vhost.conf`
- Document root under `/var/www/<domain>/html`
- WordPress files when `--wp` is enabled
- LiteSpeed Cache plugin in `wp-content/plugins/litespeed-cache` when `--wp` is enabled
- WordPress database/user + `wp-config.php` + `wp core install` when `--wp` is enabled
- Console output with WordPress admin URL, admin user, and generated admin password when `--wp` is enabled
- Credential persistence at `/etc/ols-cli/sites/<domain>/credentials.txt` (mode `0600`) when `--wp` is enabled
- Domain registration into `/usr/local/lsws/conf/httpd_config.conf` (virtualhost + listener maps for `Default` and `SSL` when present)
- Let's Encrypt certificate issuance through `certbot` (webroot challenge)
- SSL cert/key wiring into vhost `vhssl` (`certFile` and `keyFile`)

### Create a site with defaults (WordPress + PHP 8.5)

```bash
sudo ols site create example.com --wp
```

### Update a site to a target PHP version

```bash
sudo ols site update example.com --php85
```

### Update a site and ensure WordPress + LiteSpeed Cache exist

```bash
sudo ols site update example.com --wp --php83
```

### Preview operations without making changes

```bash
ols --dry-run site create example.com --wp --le --php85
ols --dry-run site update example.com --wp --php83
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
./ols --dry-run site create example.com --wp --le --php85
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

This repository now provides a practical baseline for:

- Runtime installation (`ols install`)
- Site creation with virtual host file generation
- Main OLS server config registration (virtualhost + listener map)
- WordPress provisioning with LiteSpeed Cache plugin
- Site-level PHP handler switching via `ols site update`

Planned next phases:

- Full listener selection/multi-listener mapping strategy
- Automated Let's Encrypt issuance and SSL listener wiring
- Optional OLS reload/restart automation toggles
- Full Debian repository publishing pipeline
