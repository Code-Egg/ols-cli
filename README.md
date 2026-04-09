# OLS CLI

![ols-cli](https://socialify.git.ci/Code-Egg/ols-cli/image?custom_description=A+CLI+tool+for+managing+OpenLiteSpeed+and+WordPress.&description=1&language=1&logo=https%3A%2F%2Fopenlitespeed.org%2Fwp-content%2Fuploads%2F2018%2F05%2Fopenlitespeed-logo-1.png&theme=Dark)
 
[![CI Status](https://github.com/Code-Egg/ols-cli/workflows/ci/badge.svg)](https://github.com/Code-Egg/ols-cli/actions/)
[![Release Status](https://github.com/Code-Egg/ols-cli/workflows/release/badge.svg)](https://github.com/Code-Egg/ols-cli/actions/workflows/release.yml)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Release](https://img.shields.io/github/v/release/Code-Egg/ols-cli?display_name=tag)](https://github.com/Code-Egg/ols-cli/releases)

[![Ubuntu](https://img.shields.io/badge/OS-Ubuntu-E95420?logo=ubuntu&logoColor=white)](https://ubuntu.com/)
[![CentOS](https://img.shields.io/badge/OS-CentOS-262577?logo=centos&logoColor=white)](https://www.centos.org/)
[![CPU amd64](https://img.shields.io/badge/CPU-amd64_(x86__64)-5C2D91?logo=amd&logoColor=white)](https://en.wikipedia.org/wiki/X86-64)
[![CPU arm64](https://img.shields.io/badge/CPU-arm64_(aarch64)-0091BD?logo=arm&logoColor=white)](https://en.wikipedia.org/wiki/AArch64)

## Installation

Use the one-line installer to install ols cli binary:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Code-Egg/ols-cli/master/install_olscli.sh)
```

### Install runtime once

Install OpenLiteSpeed, PHP, Database. 

```bash
sudo ols install
```

## Usage

### Create a site with WordPress + Let's Encrypt

```bash
sudo ols site create example.com --wp --le
```

### Create a wordprss site with default PHP version

```bash
sudo ols site create example.com --wp
```

### Update a site to a target PHP version

```bash
sudo ols site update example.com --php85
```

### Enable OWASP + reCAPTCHA and add security headers on a site

```bash
sudo ols site update example.com --enable-owasp --enable-recaptcha --hsts
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
ols --dry-run site create example.com --wp --le --php85 --enable-owasp --hsts
ols --dry-run site update example.com --enable-recaptcha --disable-owasp
ols --dry-run site info example.com
ols --dry-run site show example.com
ols --dry-run site list
```

## Command overview

| Command | Purpose | Common options |
| --- | --- | --- |
| `ols install` | Install/align OpenLiteSpeed runtime and related packages | `--php81` `--php82` `--php83` `--php84` `--php85` `--database` `--config` `--http-port` `--https-port` `--ssl-cert` `--ssl-key` `--no-listeners` |
| `ols site create <domain>` | Create a new site/vhost | `--wp` `--le` `--php81` `--php82` `--php83` `--php84` `--php85` `--enable-owasp` `--disable-owasp` `--enable-recaptcha` `--disable-recaptcha` `--hsts` |
| `ols site update <domain>` | Update an existing site (PHP target optional when only security flags are used) | `--wp` `--php81` `--php82` `--php83` `--php84` `--php85` `--enable-owasp` `--disable-owasp` `--enable-recaptcha` `--disable-recaptcha` `--hsts` |
| `ols site info <domain>` | Show site metadata and detected status | *(none)* |
| `ols site show <domain>` | Print OLS virtual host config (`vhconf.conf`) | *(none)* |
| `ols site list` | List managed sites discovered from OLS vhost directory | *(none)* |
| `ols site delete <domain>` | Remove site config/files and optionally keep DB | `--keep-db` |

Global options (apply to all commands): `--dry-run`, `--color`


`ols site` subcommands and options

| Subcommand | Syntax | Options |
| --- | --- | --- |
| `create` | `ols site create <domain>` | `--wp` `--le` `--php81` `--php82` `--php83` `--php84` `--php85` `--enable-owasp` `--disable-owasp` `--enable-recaptcha` `--disable-recaptcha` `--hsts` |
| `update` | `ols site update <domain>` | `--wp` (requires one of `--php81` `--php82` `--php83` `--php84` `--php85`), or security flags only: `--enable-owasp` `--disable-owasp` `--enable-recaptcha` `--disable-recaptcha` `--hsts` |
| `info` | `ols site info <domain>` | *(none)* |
| `show` | `ols site show <domain>` | *(none)* |
| `list` | `ols site list` | *(none)* |
| `delete` | `ols site delete <domain>` | `--keep-db` |


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


## FAQ

#### Custom the default config file
Use config file defaults (recommended for first-run bootstrap):

```bash
sudo mkdir -p /etc/ols-cli
sudo cp docs/install.example.json /etc/ols-cli/install.json
sudo ols install
```

`install.json` also supports `owasp_crs_version` (for example, `"owasp_crs_version": "4.21.0"`), which is used when enabling OWASP via `site create/update --enable-owasp`.

Override config values with flags when needed:

```bash
sudo ols install --config /etc/ols-cli/install.json --php85 --database mysql --http-port 80 --https-port 443
sudo ols install --no-listeners
```
