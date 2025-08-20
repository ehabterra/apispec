#!/bin/bash
set -euo pipefail

COVERAGE_FILE="coverage.txt"
README_FILE="README.md"
THRESHOLD=80  # Minimum acceptable coverage %

# Run tests and generate coverage report
go test ./... -coverprofile=$COVERAGE_FILE

# Extract coverage percentage
COVERAGE=$(go tool cover -func=$COVERAGE_FILE | grep total | awk '{print substr($3, 1, length($3)-1)}')

# Determine badge color
if (( $(echo "$COVERAGE >= 90" | bc -l) )); then
    COLOR="brightgreen"
elif (( $(echo "$COVERAGE >= 80" | bc -l) )); then
    COLOR="yellow"
else
    COLOR="red"
fi

# Enforce coverage threshold
if (( $(echo "$COVERAGE < $THRESHOLD" | bc -l) )); then
    echo "Coverage ($COVERAGE%) is below threshold ($THRESHOLD%). Failing."
    exit 1
fi

# Update badge in README.md
# Replace the first occurrence of a coverage badge line or insert if not present
BADGE_URL="https://img.shields.io/badge/coverage-${COVERAGE}%25-${COLOR}.svg"

if grep -q "img.shields.io/badge/coverage" "$README_FILE"; then
    sed -i.bak -E "s|!\[Coverage\]\(https://img.shields.io/badge/coverage-[0-9]+(\.[0-9]+)?%25-[a-z]+\.svg\)|![Coverage](${BADGE_URL})|" "$README_FILE"
else
    echo -e "\n![Coverage](${BADGE_URL})" >> "$README_FILE"
fi

echo "Coverage updated: ${COVERAGE}% (${COLOR})"
