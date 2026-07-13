#!/bin/bash
set -euo pipefail

COVERAGE_FILE="coverage.txt"
README_FILE="README.md"
THRESHOLD=80  # Minimum acceptable coverage %

# Library packages only. The cmd/* mains (notably cmd/apispecui's ~1400-line
# main.go) and the root main are CLI glue with no unit tests; counting their
# statements understates how well the testable library code is covered. This
# matches what an IDE coverage run over the library subtree reports.
LIB_PKGS=(./internal/... ./generator/... ./pkg/... ./spec/...)

# Cross-package attribution (-coverpkg): without it, go test only credits a
# package's own tests, so the generator/ fixture suite — which exercises the
# whole metadata→tracker→extractor→mapper pipeline end-to-end — counts for
# nothing in the packages it actually covers (lazytree.go reads 0% despite
# being the default engine). The badge should report what the tests execute,
# not where the test files happen to live.
LIB_COVERPKG=$(IFS=,; echo "${LIB_PKGS[*]}")

# Run tests and generate coverage report (library scope)
go test "${LIB_PKGS[@]}" -coverpkg="$LIB_COVERPKG" -coverprofile=$COVERAGE_FILE

# Extract coverage percentage
COVERAGE=$(go tool cover -func=$COVERAGE_FILE | grep total | awk '{print substr($3, 1, length($3)-1)}')

# Determine badge color on a graduated scale (codecov-style) rather than a
# three-band cliff, so the colour tracks coverage smoothly. With cross-package
# attribution the library sits in the 80s and is being ratcheted toward 95%,
# so green starts at 75 and brightgreen marks the 90s.
if (( $(echo "$COVERAGE >= 90" | bc -l) )); then
    COLOR="brightgreen"
elif (( $(echo "$COVERAGE >= 75" | bc -l) )); then
    COLOR="green"
elif (( $(echo "$COVERAGE >= 65" | bc -l) )); then
    COLOR="yellowgreen"
elif (( $(echo "$COVERAGE >= 50" | bc -l) )); then
    COLOR="yellow"
elif (( $(echo "$COVERAGE >= 40" | bc -l) )); then
    COLOR="orange"
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
