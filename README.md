# OLS CLI

[![CI Status](https://github.com/Code-Egg/ols-cli/workflows/ci/badge.svg)](https://github.com/Code-Egg/ols-cli/actions/)
[![Release Status](https://github.com/Code-Egg/ols-cli/workflows/release/badge.svg)](https://github.com/Code-Egg/ols-cli/actions/workflows/release.yml)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Release](https://img.shields.io/github/v/release/Code-Egg/ols-cli?display_name=tag)](https://github.com/Code-Egg/ols-cli/releases)


A practical CLI for managing the OpenLiteSpeed + WordPress lifecycle on Linux servers.

## Installation

### Direct binary download

Pick the correct architecture (`amd64` or `arm64`) and download directly from GitHub Releases.

**AMD64**
```bash
sudo wget -O /usr/local/bin/ols https://github.com/Code-Egg/ols-cli/releases/latest/download/ols-linux-amd64
sudo chmod +x /usr/local/bin/ols
```

**ARM64**

```bash
sudo wget -O /usr/local/bin/ols https://github.com/Code-Egg/ols-cli/releases/latest/download/ols-linux-arm64
sudo chmod +x /usr/local/bin/ols
```

**Verify Installation (Optional)**

```bash
wget https://github.com/Code-Egg/ols-cli/releases/latest/download/checksums.txt
sha256sum /usr/local/bin/ols
```

If you need reproducible installs, pin a specific tag instead of `latest`.

## Usage

This section now provides:
- a quick command overview table
- a dedicated `ols site` subcommand/option table
- practical examples underneath

### Command overview

| Command | Purpose | Common options |
| --- | --- | --- |
| `ols install` | Install/align OpenLiteSpeed runtime and related packages | `--php81` `--php82` `--php83` `--php84` `--php85` `--database` `--config` `--http-port` `--https-port` `--ssl-cert` `--ssl-key` `--no-listeners` |
| `ols site create <domain>` | Create a new site/vhost | `--wp` `--le` `--php81` `--php82` `--php83` `--php84` `--php85` |
| `ols site update <domain>` | Update an existing site (PHP target is required) | `--wp` `--php81` `--php82` `--php83` `--php84` `--php85` |
| `ols site info <domain>` | Show site metadata and detected status | *(none)* |
| `ols site show <domain>` | Print OLS virtual host config (`vhconf.conf`) | *(none)* |
| `ols site list` | List managed sites discovered from OLS vhost directory | *(none)* |
| `ols site delete <domain>` | Remove site config/files and optionally keep DB | `--keep-db` |

Global options (apply to all commands): `--dry-run`, `--color`

### `ols site` subcommands and options

| Subcommand | Syntax | Options |
| --- | --- | --- |
| `create` | `ols site create <domain>` | `--wp` `--le` `--php81` `--php82` `--php83` `--php84` `--php85` |
| `update` | `ols site update <domain>` | `--wp` and exactly one of `--php81` `--php82` `--php83` `--php84` `--php85` |
| `info` | `ols site info <domain>` | *(none)* |
| `show` | `ols site show <domain>` | *(none)* |
| `list` | `ols site list` | *(none)* |
| `delete` | `ols site delete <domain>` | `--keep-db` |

### Install runtime once (required before provisioning)

```bash
sudo ols install
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

### Show site information

```bash
ols site info example.com
```

### Show OpenLiteSpeed virtual host config

```bash
ols site show example.com
```

### List managed sites

```bash
ols site list
```

### Preview operations without making changes

```bash
ols --dry-run site create example.com --wp --le --php85
ols --dry-run site update example.com --wp --php83
ols --dry-run site info example.com
ols --dry-run site show example.com
ols --dry-run site list
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
