# Installation Guide

This guide covers all the ways to install and use apispec.

## Prerequisites

- **Go 1.24 or later** - [Download from golang.org](https://golang.org/doc/install)
- **Git** - For cloning the repository

## Installation Methods

### 1. Go Install (Recommended)

The easiest way to install apispec is using Go's built-in install command:

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

### 2. From Source

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

### 3. Using Installation Script

We provide a convenient installation script:

```bash
# Download and run the installation script
curl -sSL https://raw.githubusercontent.com/ehabterra/apispec/main/scripts/install.sh | bash -s go-install
```

**Pros:**
- Automated installation process
- Multiple installation options
- Error checking and validation

**Cons:**
- Requires curl/wget
- Downloads and executes scripts from the internet

## Platform-Specific Instructions

### macOS

```bash
# Using Homebrew (if available)
brew install ehabterra/tap/apispec

# Using Go install
go install github.com/ehabterra/apispec/cmd/apispec@latest

# From source
git clone https://github.com/ehabterra/apispec.git
cd apispec
make install-local
```

### Linux

```bash
# Using Go install
go install github.com/ehabterra/apispec/cmd/apispec@latest

# From source
git clone https://github.com/ehabterra/apispec.git
cd apispec
make install-local
```

### Windows

```bash
# Using Go install
go install github.com/ehabterra/apispec/cmd/apispec@latest

# From source
git clone https://github.com/ehabterra/apispec.git
cd apispec
go build -o apispec.exe ./cmd/apispec
# Copy apispec.exe to a directory in your PATH
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
apispec version: v1.0.0
Commit: abc123
Build date: 2024-01-01T00:00:00Z
Go version: go1.21.0
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

### Go Install Method
```bash
go install github.com/ehabterra/apispec/cmd/apispec@latest@latest
```

### From Source
```bash
cd apispec
git pull
make install-local
```

## Uninstalling

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

3. **Go version compatibility**
   - Ensure you have Go 1.24 or later
   - Run `go version` to check

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

Pre-built binaries are available for each release on the [GitHub Releases page](https://github.com/ehabterra/apispec/releases).

Supported platforms:
- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64, arm64)

Each release includes:
- Platform-specific binaries
- SHA256 checksums for verification
- Source code archives
- Release notes
