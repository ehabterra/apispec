#!/bin/bash
set -euo pipefail

# Coverage ratchet: fail when library coverage drops below the committed
# floor (scripts/coverage-floor.txt). The floor only moves up — bump it in
# the same PR that raises coverage, so improvements can't silently decay.
#
# Scope and methodology match the badge (scripts/update-coverage-badge.sh):
# library packages only, with -coverpkg so the generator/ fixture suites
# credit the internal pipeline code they exercise cross-package.

cd "$(dirname "$0")/.."

FLOOR_FILE="scripts/coverage-floor.txt"
FLOOR=$(tr -d '[:space:]' < "$FLOOR_FILE")

LIB_PKGS=(./internal/... ./generator/... ./pkg/... ./spec/...)
LIB_COVERPKG=$(IFS=,; echo "${LIB_PKGS[*]}")

COVERAGE_FILE=$(mktemp -t apispec-coverage.XXXXXX)
trap 'rm -f "$COVERAGE_FILE"' EXIT

# -count=1 bypasses cached results: a stale cache can reference files that
# no longer exist (renames) and corrupt the merged profile.
go test -count=1 "${LIB_PKGS[@]}" -coverpkg="$LIB_COVERPKG" -coverprofile="$COVERAGE_FILE"

COVERAGE=$(go tool cover -func="$COVERAGE_FILE" | grep total | awk '{print substr($3, 1, length($3)-1)}')

echo ""
echo "Library coverage: ${COVERAGE}% (floor: ${FLOOR}%)"

if (( $(echo "$COVERAGE < $FLOOR" | bc -l) )); then
    echo "FAIL: coverage dropped below the ratchet floor."
    echo "Either add tests for the code this change leaves uncovered, or —"
    echo "only if the drop is justified (e.g. deleting well-tested code) —"
    echo "lower $FLOOR_FILE in this PR and say why in the PR description."
    exit 1
fi

# Nudge, don't fail: when coverage has grown well past the floor, the floor
# should catch up so the gain is locked in.
if (( $(echo "$COVERAGE - $FLOOR >= 1.0" | bc -l) )); then
    echo "NOTE: coverage is ≥1pt above the floor — consider bumping $FLOOR_FILE to lock in the gain."
fi
