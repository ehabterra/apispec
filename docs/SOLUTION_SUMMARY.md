# Solution Summary: Fixed Test File Changes Due to Temporary Directory Names

## Problem Description

The `metadata_test.go` file was generating test output files (`*.yaml`) in the `internal/spec/tests/` directory that contained full paths to temporary directories created by `packagestest.Export()`. These temporary directory names changed on every test run (e.g., `/var/folders/.../TestGenerateMetadata_1234567890/`), causing the test files to appear "changed" on every run and creating unnecessary diffs in PRs.

## Root Cause

The issue was in the `TestGenerateMetadata` function in `internal/metadata/metadata_test.go`:

1. **Temporary Directory Creation**: `packagestest.Export()` creates temporary directories with random names
2. **File Path Storage**: The generated metadata contains file paths that include these temporary directory names
3. **Test File Writing**: The test writes this metadata to YAML files in `../../internal/spec/tests/%s.yaml`
4. **Git Tracking**: These files are tracked by git, so changes appear as diffs

## Solution Implemented


### 1. Helper Script

Created `scripts/enable-test-files.sh` to easily enable test file generation during development:

```bash
#!/bin/bash
export SWAGEN_WRITE_TEST_FILES=1
echo "Test file generation enabled. Run: go test ./internal/metadata -v"
```

### 2. Documentation Updates

Updated `internal/metadata/README.md` with clear instructions on how to use the environment variable and warnings about temporary directory paths.

## Benefits

1. **Clean PRs**: No more unnecessary file changes in pull requests
2. **Development Flexibility**: Developers can still generate test files when needed for debugging
3. **Clear Documentation**: Developers understand when and how to enable test file generation

## Usage

### Normal Testing (Default)
```bash
go test ./internal/metadata -v
# No test files generated, clean output
```

### Development Testing (Generate Files)
```bash
# Option 1: Use the helper script
./scripts/enable-test-files.sh
go test ./internal/metadata -v

# Option 2: Set environment variable manually
export SWAGEN_WRITE_TEST_FILES=1
go test ./internal/metadata -v

# Option 3: One-liner
SWAGEN_WRITE_TEST_FILES=1 go test ./internal/metadata -v
```

### Disable Test File Generation
```bash
unset SWAGEN_WRITE_TEST_FILES
# or restart your shell session
```

## Files Modified

1. **`internal/metadata/metadata_test.go`**: Added environment variable control
2. **`internal/metadata/README.md`**: Added documentation about test file generation
3. **`scripts/enable-test-files.sh`**: Helper script for developers

## Future Considerations

If you need to permanently generate test files without temporary directory paths, consider:

1. **Path Sanitization**: Implement more sophisticated path cleaning in the metadata generation
2. **Relative Paths**: Use relative paths instead of absolute paths in metadata
3. **Test Data Directory**: Create a dedicated test data directory that's not tracked by git

## Conclusion

This solution provides the best of both worlds:
- **Clean, predictable test runs** for CI/CD and normal development
- **Flexible test file generation** when developers need to inspect metadata
- **No more annoying test diffs** in pull requests

The environment variable approach is simple, effective, and follows common practices for controlling test behavior.
