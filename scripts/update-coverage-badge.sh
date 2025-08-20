#!/bin/bash

# Script to update coverage badge in README.md
# Usage: ./scripts/update-coverage-badge.sh

set -e

# Get the directory of this script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_ROOT"

echo "Running tests with coverage..."
go test -coverprofile=coverage.out ./...

echo "Extracting coverage percentage..."
COVERAGE=$(go tool cover -func=coverage.out | tail -1 | awk '{print $3}' | sed 's/%//')

echo "Coverage: $COVERAGE%"

# Create coverage badge URL
BADGE_URL="https://img.shields.io/badge/coverage-${COVERAGE}%25-brightgreen?style=flat&logo=go&logoColor=white"

echo "Badge URL: $BADGE_URL"

# Create a temporary file for the new README content
TEMP_README=$(mktemp)

# Check if coverage badge already exists
if grep -q "!\[Coverage\]" README.md; then
    echo "Updating existing coverage badge..."
    # Replace the existing badge line using a more precise pattern
    awk -v badge="![Coverage]($BADGE_URL)" '
        /!\[Coverage\]/ { print badge; next }
        { print }
    ' README.md > "$TEMP_README"
else
    echo "Adding new coverage badge..."
    # Add badge after the title line
    awk -v badge="![Coverage]($BADGE_URL)" '
        NR==1 { print; print badge; next }
        { print }
    ' README.md > "$TEMP_README"
fi

# Replace the original README with the updated version
mv "$TEMP_README" README.md

echo "README.md updated with coverage badge: $COVERAGE%"

# Clean up
rm -f coverage.out

echo "Done!"
