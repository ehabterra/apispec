#!/usr/bin/env bash
#
# compare-spec.sh — regenerate OpenAPI specs for a set of projects and either
# compare each against a saved snapshot named openapi-7.<N>.yaml (default) or
# generate a fresh snapshot version for every project (--generate).
#
# By default the project set is assembled automatically from:
#   1. every project under testdata/ (any subdir containing Go files), and
#   2. the external project paths listed in scripts/compare-spec.paths
#      (git-ignored; one path per line, resolved relative to the repo root).
# Passing PATH arguments overrides this and uses exactly those paths.
#
# Usage:
#   scripts/compare-spec.sh [options] [PATH ...]
#
# Options:
#   -v, --version N   Snapshot version -> openapi-7.<N>.yaml (e.g. -v 51).
#                     Compare mode: if omitted you are prompted to pick from the
#                     versions found. Generate mode: if omitted, the next version
#                     after the highest existing one is used.
#   -g, --generate    Generate/overwrite snapshots openapi-7.<N>.yaml for every
#                     project instead of comparing.
#   -k, --keep        Keep the freshly generated spec file (compare mode only;
#                     default: deleted).
#   -a, --all         Also report CHANGED and ADDED keys, not just MISSING.
#       --strict      Compare keys literally (do NOT canonicalize '.'<->'_' in
#                     schema names / $refs). Off by default.
#       --paths FILE  External-projects list file (default: scripts/compare-spec.paths).
#       --no-testdata Do not auto-include projects under testdata/.
#       --bin PATH    Use an existing apispec binary instead of building one.
#   -h, --help        Show this help.
#
# Exit status: non-zero if any path has missing parts, a missing snapshot, or a
# generation failure.
#
set -euo pipefail


REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GEN_NAME=".compare_gen.yaml"   # temp filename written inside each path

VERSION=""
KEEP=0
SHOW_ALL=0
STRICT=0
GENERATE=0
NO_TESTDATA=0
APISPEC_BIN=""
PATHS_FILE="$REPO_ROOT/scripts/compare-spec.paths"
PATHS=()

usage() { sed -n '2,38p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    -v|--version)   VERSION="$2"; shift 2 ;;
    -g|--generate)  GENERATE=1; shift ;;
    -k|--keep)      KEEP=1; shift ;;
    -a|--all)       SHOW_ALL=1; shift ;;
    --strict)       STRICT=1; shift ;;
    --paths)        PATHS_FILE="$2"; shift 2 ;;
    --no-testdata)  NO_TESTDATA=1; shift ;;
    --bin)          APISPEC_BIN="$2"; shift 2 ;;
    -h|--help)      usage; exit 0 ;;
    -*)             echo "Unknown option: $1" >&2; usage; exit 2 ;;
    *)              PATHS+=("$1"); shift ;;
  esac
done

# Resolve a path token to an absolute path: absolute tokens as-is, otherwise
# relative to the repo root (so the script works from any CWD).
resolve() {
  case "$1" in
    /*) printf '%s\n' "$1" ;;
    *)  printf '%s/%s\n' "$REPO_ROOT" "$1" ;;
  esac
}

# Auto-discover projects under testdata/: direct subdirs containing Go files.
discover_testdata() {
  local base="$REPO_ROOT/testdata" d name
  [[ -d "$base" ]] || return 0
  for d in "$base"/*/; do
    d="${d%/}"
    name="$(basename "$d")"
    [[ "$name" == "temp" ]] && continue
    if find "$d" -name '*.go' -print -quit 2>/dev/null | grep -q .; then
      printf 'testdata/%s\n' "$name"
    fi
  done | sort
}

# Read external project paths from the list file (strip comments/blanks/trim).
read_paths_file() {
  local f="$1" line
  [[ -f "$f" ]] || return 0
  while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line%%#*}"
    line="${line#"${line%%[![:space:]]*}"}"
    line="${line%"${line##*[![:space:]]}"}"
    [[ -n "$line" ]] && printf '%s\n' "$line"
  done < "$f"
}

# Build the default project set when no PATH arguments were given.
if [[ ${#PATHS[@]} -eq 0 ]]; then
  if [[ $NO_TESTDATA -eq 0 ]]; then
    while IFS= read -r t; do PATHS+=("$t"); done < <(discover_testdata)
  fi
  while IFS= read -r t; do PATHS+=("$t"); done < <(read_paths_file "$PATHS_FILE")
  if [[ ${#PATHS[@]} -eq 0 ]]; then
    echo "Error: no paths given, none discovered under testdata/, and none in" >&2
    echo "       $PATHS_FILE" >&2
    usage; exit 2
  fi
  echo ">> Project set: ${#PATHS[@]} paths (testdata auto-discovered + $(basename "$PATHS_FILE"))" >&2
fi

# Drop paths whose directory does not exist (external paths are machine-specific).
VALID=()
for t in "${PATHS[@]}"; do
  if [[ -d "$(resolve "$t")" ]]; then
    VALID+=("$t")
  else
    echo "  SKIP (directory not found): $t" >&2
  fi
done
PATHS=("${VALID[@]}")
if [[ ${#PATHS[@]} -eq 0 ]]; then
  echo "Error: none of the requested directories exist." >&2
  exit 2
fi

# Collect snapshot versions present across the selected paths.
all_versions() {
  local t
  for t in "${PATHS[@]}"; do
    find "$(resolve "$t")" -maxdepth 1 -name 'openapi-7.*.yaml' 2>/dev/null
  done | sed -E 's/.*openapi-7\.([0-9]+)\.yaml/\1/' | sort -n | uniq
}

# Resolve the version to use.
if [[ -z "$VERSION" ]]; then
  if [[ $GENERATE -eq 1 ]]; then
    maxv="$(all_versions | tail -1)"
    VERSION=$(( ${maxv:-0} + 1 ))
    echo ">> No version given; generating next version: 7.${VERSION}" >&2
  else
    mapfile -t AVAIL < <(all_versions)
    if [[ ${#AVAIL[@]} -eq 0 ]]; then
      echo "No openapi-7.<N>.yaml snapshots found in the selected paths." >&2
      exit 2
    fi
    echo "Available snapshot versions (openapi-7.<N>.yaml): ${AVAIL[*]}" >&2
    read -rp "Select version number to compare with: " VERSION
  fi
fi

# Normalize: accept "51" or "7.51".
VERSION="${VERSION#7.}"
if ! [[ "$VERSION" =~ ^[0-9]+$ ]]; then
  echo "Error: version must be a number (e.g. 51 or 7.51), got '$VERSION'." >&2
  exit 2
fi
SNAPSHOT="openapi-7.${VERSION}.yaml"

# Build apispec once unless a binary was supplied.
if [[ -z "$APISPEC_BIN" ]]; then
  APISPEC_BIN="$(mktemp -t apispec.XXXXXX)"
  echo ">> Building apispec ..." >&2
  ( cd "$REPO_ROOT" && go build -o "$APISPEC_BIN" ./cmd/apispec )
fi

# Run apispec for a project, honoring its used-config.yaml when present.
# Args: <abs-dir> <output-filename-relative-to-dir> <errfile>
run_apispec() {
  local abs="$1" out="$2" err="$3"
  local cfgargs=()
  [[ -f "$abs/used-config.yaml" ]] && cfgargs=(-c "$abs/used-config.yaml")
  "$APISPEC_BIN" --dir "$abs" ${cfgargs[@]+"${cfgargs[@]}"} -o "$out" >/dev/null 2>"$err"
}

# Structural comparator: flattens both specs to leaf key-PATHS (kept as tuples so
# dots inside schema names like "pkg.Type" never collide with the path separator)
# and reports keys in the snapshot that are absent from / differ in the generated
# spec. Schema component names and $ref targets are canonicalized ('.' <-> '_') so
# a cosmetic rename of the sanitizer does not masquerade as hundreds of drops.
compare_py() {
python3 - "$1" "$2" "$SHOW_ALL" "$STRICT" <<'PY'
import sys, yaml

ref_file, gen_file = sys.argv[1], sys.argv[2]
show_all = sys.argv[3] == "1"
strict   = sys.argv[4] == "1"

def flatten(obj, prefix=()):
    out = {}
    if isinstance(obj, dict):
        for k, v in obj.items():
            out.update(flatten(v, prefix + (str(k),)))
    elif isinstance(obj, list):
        for i, v in enumerate(obj):
            out.update(flatten(v, prefix + (f"[{i}]",)))
    else:
        out[prefix] = obj
    return out

def canon_seg(seg):
    # Canonicalize a single path segment: treat '.' and '_' as the same separator
    # in schema component identifiers (the sanitizer changed '.' -> '_').
    return seg.replace(".", "_")

def canon_key(key):
    # Only canonicalize the schema-name segment (child of components.schemas.*).
    if len(key) >= 3 and key[0] == "components" and key[1] == "schemas":
        return key[:2] + (canon_seg(key[2]),) + key[3:]
    return key

def canon_val(v):
    if isinstance(v, str) and v.startswith("#/components/schemas/"):
        head, name = v.rsplit("/", 1)
        return head + "/" + canon_seg(name)
    return v

def show(key):
    # Human-readable path for display.
    out = ""
    for seg in key:
        if seg.startswith("["):
            out += seg
        else:
            out += ("." if out else "") + seg
    return out

with open(ref_file) as f: ref_raw = flatten(yaml.safe_load(f) or {})
with open(gen_file) as f: gen_raw = flatten(yaml.safe_load(f) or {})

if strict:
    ref = ref_raw
    gen = gen_raw
else:
    ref = {canon_key(k): canon_val(v) for k, v in ref_raw.items()}
    gen = {canon_key(k): canon_val(v) for k, v in gen_raw.items()}

missing = sorted((k for k in ref if k not in gen), key=show)
changed = sorted((k for k in ref if k in gen and ref[k] != gen[k]), key=show)
added   = sorted((k for k in gen if k not in ref), key=show)

# Note the schema-rename normalization if it actually merged anything.
if not strict:
    renamed = sum(1 for k in ref_raw if canon_key(k) != k)
    if renamed:
        print(f"  (note: schema names canonicalized '.'<->'_'; {renamed} keys "
              f"normalized — pass --strict to compare literally)")

if missing:
    print(f"  MISSING ({len(missing)}) — in snapshot, absent from generated:")
    for k in missing:
        print(f"    - {show(k)} = {ref[k]!r}")
else:
    print("  MISSING (0) — nothing from the snapshot was dropped.")

if show_all:
    if changed:
        print(f"  CHANGED ({len(changed)}):")
        for k in changed:
            print(f"    ~ {show(k)}: {ref[k]!r} -> {gen[k]!r}")
    if added:
        print(f"  ADDED ({len(added)}) — new in generated:")
        for k in added:
            print(f"    + {show(k)} = {gen[k]!r}")

# Exit 1 if anything is missing so the wrapper can aggregate failures.
sys.exit(1 if missing else 0)
PY
}

OVERALL=0
echo

# -------- Generate mode: write openapi-7.<N>.yaml into every project. --------
if [[ $GENERATE -eq 1 ]]; then
  for t in "${PATHS[@]}"; do
    abs="$(resolve "$t")"
    echo "=============================================================="
    echo "PATH: $t"
    cfgnote=""
    [[ -f "$abs/used-config.yaml" ]] && cfgnote=" (with used-config.yaml)"
    if run_apispec "$abs" "$SNAPSHOT" "$abs/.compare_err"; then
      rm -f "$abs/.compare_err"
      echo "  WROTE ${SNAPSHOT}${cfgnote}"
    else
      echo "  GENERATION FAILED:"
      sed 's/^/    /' "$abs/.compare_err"
      rm -f "$abs/.compare_err"
      OVERALL=1
    fi
    echo
  done
  echo "=============================================================="
  if [[ $OVERALL -eq 0 ]]; then
    echo "RESULT: wrote ${SNAPSHOT} for all ${#PATHS[@]} project(s)."
  else
    echo "RESULT: some projects failed to generate ${SNAPSHOT} (see above)."
  fi
  exit $OVERALL
fi

# -------- Compare mode: diff generated spec against the saved snapshot. --------
for t in "${PATHS[@]}"; do
  abs="$(resolve "$t")"
  echo "=============================================================="
  echo "PATH: $t"
  snap="$abs/$SNAPSHOT"
  if [[ ! -f "$snap" ]]; then
    echo "  SNAPSHOT NOT FOUND: $SNAPSHOT (skipping)"
    OVERALL=1
    echo
    continue
  fi

  # Generate fresh spec into the path (apispec resolves -o relative to --dir).
  # Snapshots are produced with the dir's used-config.yaml when present, so use
  # it here too — otherwise externalTypes-mapped types (gin.H, fiber.Map, …)
  # would differ purely because the config was absent, not because of a change.
  cfgnote=""
  [[ -f "$abs/used-config.yaml" ]] && cfgnote=" (with used-config.yaml)"
  if ! run_apispec "$abs" "$GEN_NAME" "$abs/.compare_err"; then
    echo "  GENERATION FAILED:"
    sed 's/^/    /' "$abs/.compare_err"
    rm -f "$abs/.compare_err"
    OVERALL=1
    echo
    continue
  fi
  rm -f "$abs/.compare_err"

  echo "  Compared generated spec against ${SNAPSHOT}${cfgnote}"
  if ! compare_py "$snap" "$abs/$GEN_NAME"; then
    OVERALL=1
  fi

  if [[ $KEEP -eq 1 ]]; then
    echo "  (kept generated spec: $t/$GEN_NAME)"
  else
    rm -f "$abs/$GEN_NAME"
  fi
  echo
done

echo "=============================================================="
if [[ $OVERALL -eq 0 ]]; then
  echo "RESULT: no missing parts across all paths."
else
  echo "RESULT: missing parts and/or missing snapshots found (see above)."
fi
exit $OVERALL
