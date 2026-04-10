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

### Create a wordprss site

```bash
sudo ols site create example.com --wp
```

### Create a site with WordPress + Let's Encrypt

```bash
sudo ols site create example.com --wp --le
```

### Update a site to a target PHP version

```bash
sudo ols site update example.com --php85
```

### Update CLI binary to latest release

```bash
sudo ols update
```

### Security

#### Enable OWASP + reCAPTCHA and add security headers on a site

```bash
sudo ols site update example.com --enable-owasp --enable-recaptcha --hsts
```

#### Enable namespace

```bash
sudo ols site update example.com --enable-ns
```


### Control 

#### Show site information

```bash
ols site info example.com
```

#### Show OpenLiteSpeed virtual host config

```bash
ols site show example.com
```

#### List managed sites

```bash
ols site list
```

### Preview mode

```bash
ols --dry-run site create example.com --wp --le --php85 --enable-owasp --hsts --enable-ns
ols --dry-run site update example.com --enable-recaptcha --disable-owasp --disable-ns --le
ols --dry-run site info example.com
ols --dry-run site show example.com
ols --dry-run site list
ols --dry-run update
```

## Command overview

The ols commands:

```bash
ols (command) [options]
```

| Command | Purpose | Options |
| --- | --- | --- |
| `install` | Install/align OpenLiteSpeed runtime and related packages | `--php81` `--php82` `--php83` `--php84` `--php85` `--database` `--config` `--http-port` `--https-port` `--ssl-cert` `--ssl-key` `--no-listeners` |
| `site` | Manage sites (`create`, `update`, `enable`, `disable`, `info`, `show`, `list`, `delete`) | `--wp` `--le` `--php81` `--php82` `--php83` `--php84` `--php85` `--enable-owasp` `--disable-owasp` `--enable-recaptcha` `--disable-recaptcha` `--enable-ns` `--disable-ns` `--hsts` `--keep-db` |
| `update` | Update installed `ols` binary to latest GitHub release for current platform |  |

Global options (apply to all commands): `--dry-run`, `--color`


### site
```bash
ols site (command) [options]
```

| Subcommand | Purpose | Options |
| --- | --- | --- |
| `create` | Create a new site/vhost | `--wp` `--le` `--php81` `--php82` `--php83` `--php84` `--php85` `--enable-owasp` `--disable-owasp` `--enable-recaptcha` `--disable-recaptcha` `--enable-ns` `--disable-ns` `--hsts` |
| `update` | Update an existing site (PHP target optional when only security/LE flags are used) | `--wp` (requires one of `--php81` `--php82` `--php83` `--php84` `--php85`), optional `--le`, or security flags only: `--enable-owasp` `--disable-owasp` `--enable-recaptcha` `--disable-recaptcha` `--enable-ns` `--disable-ns` `--hsts` |
| `enable` | Enable site by removing domain from server-level `suspendedVhosts` |  |
| `disable` | Disable site by adding domain to server-level `suspendedVhosts` |  |
| `info` | Show site metadata and detected status |  |
| `show` | Print OLS virtual host config (`vhconf.conf`) |  |
| `list` | List managed sites discovered from OLS vhost directory |  |
| `delete` | Remove site config/files and optionally keep DB | `--keep-db` |


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

`install.json` also supports:
- `owasp_crs_version` (for example, `"4.21.0"`) for OWASP CRS setup during `ols install`
- `vh_recaptcha_type` and `vh_recaptcha_reg_conn_limit` (defaults: `1` and `500`) used when enabling vhost reCAPTCHA via `site create/update --enable-recaptcha`

By default, `ols install` prepares server-level security blocks as:
- `module mod_security` with `ls_enabled 0`
- `lsrecaptcha` with `enabled 1` and `type 0`
- `namespace` with `1`

Override config values with flags when needed:

```bash
sudo ols install --config /etc/ols-cli/install.json --php85 --database mysql --http-port 80 --https-port 443
sudo ols install --no-listeners
```
