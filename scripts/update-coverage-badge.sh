#!/bin/bash
set -euo pipefail

# Computes library test coverage and writes a shields.io "endpoint" badge file
# (coverage.json). The CI workflow publishes that file to the unprotected
# `badges` branch, and the README badge reads it from there — so the coverage
# badge never requires a commit to the protected main branch.

COVERAGE_FILE="coverage.txt"
BADGE_FILE="coverage.json"
THRESHOLD=45 # Minimum acceptable coverage %

# Library packages only. The cmd/* mains (notably cmd/apispecui's ~1400-line
# main.go) and the root main are CLI glue with no unit tests; counting their
# statements understates how well the testable library code is covered. This
# matches what an IDE coverage run over the library subtree reports.
LIB_PKGS=(./internal/... ./generator/... ./pkg/... ./spec/...)

# Run tests and generate coverage report (library scope)
go test "${LIB_PKGS[@]}" -coverprofile=$COVERAGE_FILE

# Extract coverage percentage
COVERAGE=$(go tool cover -func=$COVERAGE_FILE | grep total | awk '{print substr($3, 1, length($3)-1)}')

# Determine badge color on a graduated scale (codecov-style) rather than a
# three-band cliff, so the colour tracks coverage smoothly. For a tool whose
# CLI/UI glue is intentionally left untested, library coverage in the 65–80%
# range is healthy and should read green-ish, not alarming red.
if (($(echo "$COVERAGE >= 90" | bc -l))); then
	COLOR="brightgreen"
elif (($(echo "$COVERAGE >= 75" | bc -l))); then
	COLOR="green"
elif (($(echo "$COVERAGE >= 65" | bc -l))); then
	COLOR="yellowgreen"
elif (($(echo "$COVERAGE >= 50" | bc -l))); then
	COLOR="yellow"
elif (($(echo "$COVERAGE >= 40" | bc -l))); then
	COLOR="orange"
else
	COLOR="red"
fi

# Enforce coverage threshold
if (($(echo "$COVERAGE < $THRESHOLD" | bc -l))); then
	echo "Coverage ($COVERAGE%) is below threshold ($THRESHOLD%). Failing."
	exit 1
fi

# Write the shields.io endpoint badge data.
cat >"$BADGE_FILE" <<JSON
{"schemaVersion":1,"label":"coverage","message":"${COVERAGE}%","color":"${COLOR}"}
JSON

echo "Coverage: ${COVERAGE}% (${COLOR}) -> ${BADGE_FILE}"
