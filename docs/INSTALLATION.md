# Installation Guide

This guide covers all the ways to install and use apispec.

## Prerequisites

**None** for the pre-built binaries below — they are self-contained.

For the `go install` and from-source methods:

- **Go 1.26 or later** — [Download from golang.org](https://golang.org/doc/install) (the module declares `go 1.26.0`)
- **Git** — for cloning the repository

## Installation Methods

### 1. Homebrew (macOS and Linux)

```bash
brew install ehabterra/tap/apispec
```

Upgrade with `brew upgrade apispec`, remove with `brew uninstall apispec`.
Homebrew picks the right build for your machine and puts `apispec` on your PATH.

### 2. Download a Pre-built Binary

No Go toolchain and no Homebrew required. **Copy the whole block** — it detects
your platform, verifies the checksum, and installs `apispec` onto your PATH.

**macOS / Linux**

```bash
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in x86_64|amd64) ARCH=amd64 ;; arm64|aarch64) ARCH=arm64 ;; esac
ASSET="apispec-${OS}-${ARCH}"
BASE="https://github.com/ehabterra/apispec/releases/latest/download"

curl -fsSL -O "$BASE/$ASSET"
curl -fsSL -O "$BASE/$ASSET.sha256"
shasum -a 256 -c "$ASSET.sha256" 2>/dev/null || sha256sum -c "$ASSET.sha256"

sudo install -m 0755 "$ASSET" /usr/local/bin/apispec
rm -f "$ASSET" "$ASSET.sha256"

apispec --version
```

The checksum step prints `apispec-darwin-arm64: OK` (or your platform's name). If
it prints `FAILED`, stop — do not install the file.

> The binary is **downloaded under the asset's own name** because the published
> `.sha256` file names the asset; `curl -o apispec` would rename it out from
> under the check. The `install` line is what gives it the short name and puts it
> on your PATH — the `rm` afterwards is why nothing is left in your working
> directory.

Installing somewhere else, e.g. no `sudo`:

```bash
mkdir -p ~/.local/bin && install -m 0755 "$ASSET" ~/.local/bin/apispec
# ensure ~/.local/bin is on your PATH
```

**Windows (PowerShell)**

```powershell
$asset = "apispec-windows-amd64.exe"   # or apispec-windows-arm64.exe on ARM
$base  = "https://github.com/ehabterra/apispec/releases/latest/download"

Invoke-WebRequest -Uri "$base/$asset" -OutFile $asset
Invoke-WebRequest -Uri "$base/$asset.sha256" -OutFile "$asset.sha256"
$expected = (Get-Content "$asset.sha256").Split(" ")[0]
if ((Get-FileHash $asset -Algorithm SHA256).Hash -ne $expected) { throw "checksum mismatch" }

New-Item -ItemType Directory -Force "$env:LOCALAPPDATA\Programs\apispec" | Out-Null
Move-Item -Force $asset "$env:LOCALAPPDATA\Programs\apispec\apispec.exe"
# add that directory to your PATH, then:
apispec --version
```

To pin a version, swap `latest/download` for `download/v0.5.6` (any tag).

**Pros:**
- No Go toolchain needed
- Exact, reproducible version with a published checksum

**Cons:**
- Manual updates (re-run the block to upgrade)
- Only `apispec` is published as a binary — `apispecui` and `apidiag` are built from source

### 3. Go Install

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

### 4. From Source

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

### 5. Using Installation Script

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

### macOS

```bash
brew install ehabterra/tap/apispec
```

Or use the copy-paste block in [Installation Method 2](#2-download-a-pre-built-binary), which
detects Apple Silicon vs Intel for you.

macOS may quarantine a downloaded binary (Homebrew installs are unaffected). If
Gatekeeper blocks it:

```bash
xattr -d com.apple.quarantine /usr/local/bin/apispec
```

### Linux

```bash
brew install ehabterra/tap/apispec        # if you use Homebrew on Linux
```

Otherwise use the copy-paste block in [Installation Method 2](#2-download-a-pre-built-binary),
which detects amd64 vs arm64 for you.

### Windows

Use the PowerShell block in [Installation Method 2](#2-download-a-pre-built-binary), or
`go install github.com/ehabterra/apispec/cmd/apispec@latest` if you have Go.

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

### Homebrew
```bash
brew upgrade apispec
```

### Pre-built Binary
Re-run the install block — it always fetches the newest release.

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

### Homebrew
```bash
brew uninstall apispec
```

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
