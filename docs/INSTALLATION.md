# Installation Guide

This guide covers all the ways to install and use apispec.

## Prerequisites

**None** for the pre-built binaries below — they are self-contained.

For the `go install` and from-source methods:

- **Go 1.26 or later** — [Download from golang.org](https://golang.org/doc/install) (the module declares `go 1.26.0`)
- **Git** — for cloning the repository

## Installation Methods

### 1. Download a Pre-built Binary (Recommended)

No Go toolchain required. Every release publishes a binary per platform, each with a SHA256 checksum.

**macOS (Apple Silicon)**

```bash
curl -L -O https://github.com/ehabterra/apispec/releases/latest/download/apispec-darwin-arm64
chmod +x apispec-darwin-arm64 && sudo install -m 0755 apispec-darwin-arm64 /usr/local/bin/apispec
```

**macOS (Intel)** — replace `darwin-arm64` with `darwin-amd64`.

**Linux (x86_64)**

```bash
curl -L -O https://github.com/ehabterra/apispec/releases/latest/download/apispec-linux-amd64
chmod +x apispec-linux-amd64 && sudo install -m 0755 apispec-linux-amd64 /usr/local/bin/apispec
```

**Linux (arm64)** — replace `linux-amd64` with `linux-arm64`.

**Windows (PowerShell)**

```powershell
Invoke-WebRequest -Uri https://github.com/ehabterra/apispec/releases/latest/download/apispec-windows-amd64.exe -OutFile apispec.exe
# then move apispec.exe somewhere on your PATH
```

Windows on ARM: use `apispec-windows-arm64.exe`.

**Verify before installing** (recommended). Download the binary under its
original name — the checksum file names the asset, so `curl -o apispec` would
rename it out from under the check:

```bash
curl -L -O https://github.com/ehabterra/apispec/releases/latest/download/apispec-darwin-arm64
curl -L -O https://github.com/ehabterra/apispec/releases/latest/download/apispec-darwin-arm64.sha256

shasum -a 256 -c apispec-darwin-arm64.sha256      # macOS
sha256sum -c apispec-linux-amd64.sha256           # Linux
```

Expect `apispec-darwin-arm64: OK`. Then install it under the name you want, as
the commands above do with `install -m 0755 … /usr/local/bin/apispec`.

To pin a version, swap `latest/download` for `download/v0.5.6` (any tag).

**Pros:**
- No Go toolchain needed
- Exact, reproducible version with a published checksum

**Cons:**
- Manual updates (re-download to upgrade)
- Only `apispec` is published as a binary — `apispecui` and `apidiag` are built from source

### 2. Go Install

If you already have Go:

```bash
go install github.com/ehabterra/apispec/cmd/apispec@latest
```

**Pros:**
- Simple one-liner
- Automatically updates when you run it again
- No need to manage build artifacts

**Cons:**
- Requires Go to be installed
- Binary is stored in Go's module cache

### 3. From Source

If you want to build from source or contribute to the project:

```bash
# Clone the repository
git clone https://github.com/ehabterra/apispec.git
cd apispec

# Install to user directory (no sudo required)
make install-local

# OR install to system directory (requires sudo)
make install
```

**Pros:**
- Full control over the build process
- Can modify and customize
- Good for development

**Cons:**
- More complex setup
- Need to manually update

### 4. Using Installation Script

We provide a convenient installation script:

```bash
# Download and run the installation script
curl -sSL https://raw.githubusercontent.com/ehabterra/apispec/main/scripts/install.sh | bash -s go-install
```

Supported arguments: `go-install` (default), `source-local`, `source-system`, `help`.

**Pros:**
- Automated, with error checking and validation

**Cons:**
- Requires curl/wget, and downloads and executes a script from the internet
- **Every mode requires Go** — the script builds from source and does not download
  a pre-built binary. Use method 1 if you have no Go toolchain.

## Platform-Specific Instructions

> There is no Homebrew tap. `brew install apispec` will not work.

### macOS

```bash
# Pre-built binary (Apple Silicon; use darwin-amd64 on Intel)
curl -L -O https://github.com/ehabterra/apispec/releases/latest/download/apispec-darwin-arm64
chmod +x apispec-darwin-arm64 && sudo install -m 0755 apispec-darwin-arm64 /usr/local/bin/apispec

# Or with Go
go install github.com/ehabterra/apispec/cmd/apispec@latest
```

macOS may quarantine a downloaded binary. If Gatekeeper blocks it:

```bash
xattr -d com.apple.quarantine /usr/local/bin/apispec
```

### Linux

```bash
# Pre-built binary (use linux-arm64 on arm64)
curl -L -O https://github.com/ehabterra/apispec/releases/latest/download/apispec-linux-amd64
chmod +x apispec-linux-amd64 && sudo install -m 0755 apispec-linux-amd64 /usr/local/bin/apispec

# Or with Go
go install github.com/ehabterra/apispec/cmd/apispec@latest
```

### Windows

```powershell
# Pre-built binary (use windows-arm64.exe on ARM)
Invoke-WebRequest -Uri https://github.com/ehabterra/apispec/releases/latest/download/apispec-windows-amd64.exe -OutFile apispec.exe

# Or with Go
go install github.com/ehabterra/apispec/cmd/apispec@latest
```

## Setting Up PATH

After installation, make sure the apispec binary is in your PATH:

### Linux/macOS

Add this to your shell profile (`~/.bashrc`, `~/.zshrc`, etc.):

```bash
export PATH=$HOME/go/bin:$PATH
```

### Windows

Add the Go bin directory to your system PATH or use the full path to the binary.

## Verification

Verify the installation:

```bash
apispec --version
```

You should see output like:

**When installed from a tagged release:**
```
apispec - Copyright 2026 Ehab Terra
apispec version: 0.5.6
Commit: 37ef463
Build date: 2026-08-08T05:10:14Z
Go version: go1.26.0
```

**When installed via `go install` from latest main:**
```
apispec version: v1.0.1-0.20240101120000-abc123def456
Commit: abc123d
Build date: 2024-01-01T12:00:00Z
Go version: go1.21.0
```

**When installed via `go install` without VCS info:**
```
apispec version: latest (go install)
Commit: unknown
Build date: unknown
Go version: go1.21.0
```

> **Note:** Version information depends on how `apispec` was built. When using `go install`, Go automatically embeds VCS information when available, providing accurate version details.

## Updating

### Pre-built Binary
Re-download it — the same command as installation, which always fetches the newest release.

### Go Install Method
```bash
go install github.com/ehabterra/apispec/cmd/apispec@latest
```

### From Source
```bash
cd apispec
git pull
make install-local
```

## Uninstalling

### Pre-built Binary
```bash
sudo rm /usr/local/bin/apispec
```

### Go Install Method
```bash
go clean -i github.com/ehabterra/apispec/cmd/apispec
```

### From Source
```bash
# If installed locally
make uninstall-local

# If installed system-wide
make uninstall
```

## Troubleshooting

### Common Issues

1. **"command not found: apispec"**
   - Check if the binary is in your PATH
   - Verify the installation location
   - Restart your terminal after PATH changes

2. **Permission denied errors**
   - Use `make install-local` instead of `make install`
   - Check file permissions
   - Ensure you have write access to the target directory

3. **Go version compatibility** (source / `go install` only)
   - Ensure you have Go 1.26 or later — the module declares `go 1.26.0`
   - Run `go version` to check
   - Not applicable to the pre-built binaries, which bundle their runtime

4. **Build failures**
   - Ensure all dependencies are installed
   - Run `go mod download` and `go mod tidy`
   - Check Go environment variables

### Getting Help

If you encounter issues:

1. Check the [GitHub Issues](https://github.com/ehabterra/apispec/issues)
2. Review the [README.md](../README.md) for usage examples
3. Check the [Go documentation](https://golang.org/doc/) for Go-related issues

## Development Installation

For developers who want to work on apispec:

```bash
git clone https://github.com/ehabterra/apispec.git
cd apispec

# Install dependencies
make deps

# Build for development
make build

# Run tests
make test

# Build for multiple platforms
make release
```

## Release Downloads

Every release on the [GitHub Releases page](https://github.com/ehabterra/apispec/releases) publishes these assets — see [Installation Method 1](#1-download-a-pre-built-binary-recommended) for the commands.

| Platform | Asset |
|---|---|
| macOS arm64 (Apple Silicon) | `apispec-darwin-arm64` |
| macOS amd64 (Intel) | `apispec-darwin-amd64` |
| Linux amd64 | `apispec-linux-amd64` |
| Linux arm64 | `apispec-linux-arm64` |
| Windows amd64 | `apispec-windows-amd64.exe` |
| Windows arm64 | `apispec-windows-arm64.exe` |

Each binary ships a matching `<asset>.sha256` checksum file, plus a source archive (`apispec-<version>.tar.gz`) and release notes.

Only the `apispec` CLI is published as a binary. `apispecui` (browser config & preview) and `apidiag` (call-graph server) are built from source — see [Development Installation](#development-installation).
