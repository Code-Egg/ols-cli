# APT Packaging Scaffold

This directory is a scaffold for Debian/Ubuntu package artifacts.

## Target package

- Package name: `ols`
- Installs binary to: `/usr/local/bin/ols`
- Depends on: `ca-certificates`, `curl`

## Suggested pipeline steps

1. Build release tarball: `ols-linux-amd64.tar.gz` / `ols-linux-arm64.tar.gz`
2. Build `.deb` package with architecture-specific payload.
3. Sign package and publish to apt repository.
4. Provide `apt install ols` to install/update the same signed binary.

## Notes

- Keep runtime logic in Go binary, not maintainer scripts.
- Use maintainer scripts only for minimal integration (permissions, symlink if needed).
