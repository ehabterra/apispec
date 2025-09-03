# Release Workflow

This document explains how to create releases for swagen using the automated GitHub Actions workflow.

## Overview

The release process is fully automated through GitHub Actions. When you create and push a git tag, it automatically:

1. **Extracts version information** from the tag
2. **Updates source files** with the new version
3. **Builds binaries** for multiple platforms
4. **Creates a GitHub release** with all assets
5. **Uploads release assets** (binaries, checksums, archives)

## Quick Start

### 1. Create a Release Tag

```bash
# Using the Makefile (recommended)
make create-tag VERSION=1.0.0

# Or manually
./scripts/create-release.sh 1.0.0 "Major feature release"
```

### 2. Push the Tag

```bash
git push origin v1.0.0
```

### 3. Monitor the Workflow

- Go to [Actions](https://github.com/ehabterra/swagen/actions) in your GitHub repository
- The "Release" workflow will automatically start
- Monitor the progress and check for any errors

## How It Works

### GitHub Actions Workflow (`.github/workflows/release.yml`)

The workflow triggers when you push a tag matching the pattern `v*` (e.g., `v1.0.0`).

#### Step 1: Extract Version Information
```yaml
- name: Extract version from tag
  id: version
  run: |
    VERSION=${GITHUB_REF#refs/tags/}
    VERSION=${VERSION#v}  # Remove 'v' prefix
    COMMIT=$(git rev-parse --short HEAD)
    BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    GO_VERSION=$(go version | awk '{print $3}')
```

#### Step 2: Update Source Files
```yaml
- name: Update version in Makefile
  run: |
    sed -i "s/^VERSION = .*/VERSION = ${{ steps.version.outputs.version }}/" Makefile

- name: Update version constants in main.go
  run: |
    sed -i "s/Version   = \"[^\"]*\"/Version   = \"${{ steps.version.outputs.version }}\"/" cmd/swagen/main.go
    sed -i "s/Commit    = \"[^\"]*\"/Commit    = \"${{ steps.version.outputs.commit }}\"/" cmd/swagen/main.go
    sed -i "s/BuildDate = \"[^\"]*\"/BuildDate = \"${{ steps.version.outputs.build_date }}\"/" cmd/swagen/main.go
    sed -i "s/GoVersion = \"[^\"]*\"/GoVersion = \"${{ steps.version.outputs.go_version }}\"/" cmd/swagen/main.go
```

#### Step 3: Build and Release
```yaml
- name: Build for multiple platforms
  run: make release

- name: Create Release
  uses: softprops/action-gh-release@v1
  with:
    files: |
      swagen-${{ github.ref_name }}.tar.gz
      dist/swagen-linux-amd64
      dist/swagen-linux-arm64
      dist/swagen-darwin-amd64
      dist/swagen-darwin-arm64
      dist/swagen-windows-amd64.exe
      dist/swagen-windows-arm64.exe
      dist/*.sha256
```

### Version Injection

The build process injects version information using Go's `-ldflags`:

```makefile
LDFLAGS = -X 'main.Version=$(VERSION)' \
          -X 'main.Commit=$(COMMIT)' \
          -X 'main.BuildDate=$(BUILD_DATE)' \
          -X 'main.GoVersion=$(GO_VERSION)'
```

This updates the constants in `cmd/swagen/main.go`:

```go
const (
    Version   = "1.0.0"        // Injected at build time
    Commit    = "abc1234"      // Git commit hash
    BuildDate = "2025-01-01T12:00:00Z"  // Build timestamp
    GoVersion = "go1.24.3"     // Go version used
)
```

## Supported Platforms

The workflow builds binaries for:

- **Linux**: AMD64, ARM64
- **macOS**: AMD64, ARM64
- **Windows**: AMD64, ARM64

## Release Assets

Each release includes:

1. **Platform-specific binaries** (e.g., `swagen-linux-amd64`)
2. **Checksums** (`.sha256` files for verification)
3. **Release archive** (`.tar.gz` containing all binaries)
4. **Release notes** (auto-generated from commits)

## Local Development

### Building with Custom Version

```bash
# Build with specific version
make VERSION=1.0.0 build

# Build with all custom values
make VERSION=1.0.0 COMMIT=abc123 BUILD_DATE=2025-01-01T00:00:00Z build
```

### Testing Release Process

```bash
# Create a test tag (locally only)
make create-tag VERSION=0.0.0-test

# Build release package
make release

# Clean up
git tag -d v0.0.0-test
```

## Best Practices

### 1. Semantic Versioning
Use [semantic versioning](https://semver.org/):
- `MAJOR.MINOR.PATCH` (e.g., `1.2.3`)
- `MAJOR`: Breaking changes
- `MINOR`: New features (backward compatible)
- `PATCH`: Bug fixes (backward compatible)

### 2. Tag Naming
- Always prefix with `v`: `v1.0.0`
- Use descriptive commit messages for tags
- Avoid pre-release tags in main branch

### 3. Pre-release Testing
- Test builds locally before tagging
- Use `make test` to ensure all tests pass
- Verify the binary works as expected

### 4. Release Notes
- Write clear, descriptive commit messages
- Use conventional commits for better auto-generation
- Review auto-generated release notes before publishing

## Troubleshooting

### Common Issues

#### 1. Workflow Fails on Version Update
- Check that the tag format is correct (`v1.0.0`)
- Ensure the source files exist and are writable
- Verify the sed commands work on the target platform

#### 2. Build Failures
- Check Go version compatibility
- Verify all dependencies are available
- Check for platform-specific build issues

#### 3. Asset Upload Failures
- Verify GitHub token permissions
- Check file sizes and GitHub limits
- Ensure all required files exist

### Debugging

#### Enable Debug Output
```yaml
- name: Debug information
  run: |
    echo "Version: ${{ steps.version.outputs.version }}"
    echo "Commit: ${{ steps.version.outputs.commit }}"
    echo "Build Date: ${{ steps.version.outputs.build_date }}"
    echo "Go Version: ${{ steps.version.outputs.go_version }}"
```

#### Check Generated Files
```yaml
- name: Verify version updates
  run: |
    echo "Makefile VERSION:"
    grep "^VERSION = " Makefile
    echo "main.go Version:"
    grep "Version   = " cmd/swagen/main.go
```

## Manual Release Process

If you need to create a release manually:

1. **Update version manually** in source files
2. **Build binaries** for all platforms
3. **Create GitHub release** manually
4. **Upload assets** manually

However, the automated workflow is recommended for consistency and reliability.

## Future Enhancements

Potential improvements to consider:

- **Pre-release builds** for testing
- **Docker image builds** and publishing
- **Homebrew formula updates**
- **Changelog generation** from commits
- **Release signing** for security
- **Multi-architecture Docker builds**

## Support

If you encounter issues with the release workflow:

1. Check the [Actions](https://github.com/ehabterra/swagen/actions) tab
2. Review the workflow logs for errors
3. Verify your tag format and naming
4. Ensure you have proper permissions
5. Check for any recent changes to the workflow

The automated release workflow makes it easy to maintain consistent, professional releases with minimal manual effort.
