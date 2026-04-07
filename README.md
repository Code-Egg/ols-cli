# OLS CLI

A practical CLI for managing the OpenLiteSpeed + WordPress lifecycle on Linux servers.

It is currently focused on clear command behavior, predictable output, and safe automation patterns.

## Installation

### 1) Direct binary download (recommended)

Pick the correct architecture (`amd64` or `arm64`) and download directly from GitHub Releases.

```bash
sudo wget -O /usr/local/bin/ols https://github.com/Code-Egg/ols-cli/releases/latest/download/ols-linux-amd64
sudo chmod +x /usr/local/bin/ols
```

For ARM64 servers:

```bash
sudo wget -O /usr/local/bin/ols https://github.com/Code-Egg/ols-cli/releases/latest/download/ols-linux-arm64
sudo chmod +x /usr/local/bin/ols
```

Optional checksum verification:

```bash
wget https://github.com/Code-Egg/ols-cli/releases/latest/download/checksums.txt
sha256sum /usr/local/bin/ols
```

If you need reproducible installs, pin a specific tag instead of `latest`.

## Usage

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
