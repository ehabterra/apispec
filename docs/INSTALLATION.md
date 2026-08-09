# Installation Guide

Install, update and remove apispec — four ways to do it, and how to tell which
one you are running when more than one is installed.

## At a glance

Pick a row and stay in it: the commands are not interchangeable, and installing a
second way does not replace the first (see
[Switching between methods](#switching-between-installation-methods)).

| | Install | Update | Uninstall |
|---|---|---|---|
| **[Homebrew](#1-homebrew-macos-and-linux)** — macOS/Linux, no Go needed | `brew install ehabterra/tap/apispec` | `brew upgrade apispec` | `brew uninstall apispec` |
| **[Pre-built binary](#2-download-a-pre-built-binary)** — any platform, no Go needed | [copy-paste block](#2-download-a-pre-built-binary) | re-run the same block | `sudo rm /usr/local/bin/apispec` |
| **[Go install](#3-go-install)** — needs Go 1.26+ | `go install github.com/ehabterra/apispec/cmd/apispec@latest` | same command again | `rm "$(go env GOPATH)/bin/apispec"` |
| **[From source](#4-from-source)** — for development | `make install-local` | `git pull && make install-local` | `make uninstall-local` |

Not sure what you have? → [Which apispec am I running?](#which-apispec-am-i-running)

Only the `apispec` CLI is distributed as a binary. `apispecui` (browser config &
preview) and `apidiag` (call-graph server) are built from source — see
[Development Installation](#development-installation).

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

## Which apispec am I running?

Do this before updating or uninstalling anything. More than one copy can be
installed at once, and the **first on your PATH wins** — so an upgrade can appear
to do nothing while a stale copy keeps answering.

```bash
which -a apispec     # every copy, in the order your shell searches
apispec --version    # the one that actually runs
```

Typical locations:

| path | came from |
|---|---|
| `$(brew --prefix)/bin/apispec` | Homebrew (a symlink into `Cellar/`) |
| `/usr/local/bin/apispec` | the pre-built binary block, or `make install` |
| `$(go env GOPATH)/bin/apispec` | `go install`, or `make install-local` |

If `apispec --version` disagrees with the version you just installed, you have
more than one — see [Switching between methods](#switching-between-installation-methods).

## Updating

### Homebrew
```bash
brew upgrade apispec
```

### Pre-built Binary
Re-run the install block from [Method 2](#2-download-a-pre-built-binary); it always
fetches the newest release and overwrites in place.

### Go Install
```bash
go install github.com/ehabterra/apispec/cmd/apispec@latest
```

### From Source
```bash
cd apispec && git pull && make install-local
```

## Uninstalling

Each method has to be undone its own way — removing one does not remove another.

### Homebrew
```bash
brew uninstall apispec
brew untap ehabterra/tap    # optional, if you want the tap gone too
```

### Pre-built Binary
```bash
sudo rm /usr/local/bin/apispec      # or wherever you installed it
```

### Go Install
```bash
rm "$(go env GOPATH)/bin/apispec"
```

> `go clean -i github.com/ehabterra/apispec/cmd/apispec` is **not** a reliable
> uninstall: run outside a module it fails with "go.mod file not found", which is
> the normal situation after `go install …@latest`. Remove the binary directly.

### From Source
```bash
make uninstall-local    # if installed to ~/go/bin
make uninstall          # if installed system-wide
```

## Switching between installation methods

Installing a second way does not replace the first — it just adds another copy,
and the PATH order decides which one you get. Two symptoms:

**`apispec --version` shows the old version.** An earlier copy is ahead on your
PATH. Find them all and remove the ones you do not want:

```bash
which -a apispec
rm "$(go env GOPATH)/bin/apispec"     # e.g. a stale go install build
sudo rm /usr/local/bin/apispec        # e.g. an earlier manual install
```

**Homebrew installed it but `apispec` is still the old one.** Homebrew will not
overwrite a file it does not own, so if `/usr/local/bin/apispec` already exists
as a plain file, the formula installs into `Cellar/` but never gets linked:

```bash
brew list --versions apispec          # installed?
brew link apispec                     # reports the conflicting path
sudo rm /usr/local/bin/apispec        # remove it, then
brew link apispec
```

`brew link --overwrite apispec` does the removal for you; run it with
`--dry-run` first to see what it would delete.

To go the other way — from Homebrew back to a manual install — `brew uninstall
apispec` first, or the manual binary will be the one that gets shadowed.

## Troubleshooting

### Common Issues

1. **"command not found: apispec"**
   - Check if the binary is in your PATH
   - Verify the installation location
   - Restart your terminal after PATH changes

2. **`apispec --version` shows a version you did not install**
   - More than one copy is installed; the first on your PATH wins
   - `which -a apispec` lists them all — see
     [Switching between methods](#switching-between-installation-methods)

3. **Permission denied errors**
   - Use `make install-local` instead of `make install`
   - Check file permissions
   - Ensure you have write access to the target directory

4. **Go version compatibility** (source / `go install` only)
   - Ensure you have Go 1.26 or later — the module declares `go 1.26.0`
   - Run `go version` to check
   - Not applicable to the pre-built binaries, which bundle their runtime

5. **Build failures**
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

Every release on the [GitHub Releases page](https://github.com/ehabterra/apispec/releases) publishes these assets — see [Installation Method 2](#2-download-a-pre-built-binary) for the commands.

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
