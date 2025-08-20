#!/bin/bash
set -euo pipefail

COVERAGE_FILE="coverage.txt"
README_FILE="README.md"
THRESHOLD=45  # Minimum acceptable coverage %   

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

# Prepare badge URL
BADGE_URL="https://img.shields.io/badge/coverage-${COVERAGE}%25-${COLOR}.svg"

# Update or insert badge
if grep -q "img.shields.io/badge/coverage" "$README_FILE"; then
    echo "Updating coverage badge..."
    sed -i.bak -E "s|!\[Coverage\]\(https://img.shields.io/badge/coverage-[0-9]+(\.[0-9]+)?%25-[a-z]+\.svg\)|![Coverage](${BADGE_URL})|" "$README_FILE"
else
    echo "Adding new coverage badge after the title..."
    # Insert badge after first title line (starts with '# ')
    awk -v badge="![Coverage](${BADGE_URL})" '
    NR==1 {print; print ""; print badge; next} {print}
    ' "$README_FILE" > "${README_FILE}.tmp" && mv "${README_FILE}.tmp" "$README_FILE"
fi

echo "Coverage updated: ${COVERAGE}% (${COLOR})"
