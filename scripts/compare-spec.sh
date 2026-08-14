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
#   -a, --all         Also list ADDED keys (new routes/fields). STATUS CHANGES,
#                     MISSING and CHANGED are always reported and always fail.
#       --self-test   Check the comparator's own key semantics against built-in
#                     cases and exit. Compares nothing and needs no project.
#       --compare-files REF GEN
#                     Compare two spec files directly and exit, skipping
#                     generation. Used by --self-test; also handy by hand.
#       --strict      Compare keys literally (do NOT canonicalize '.'<->'_' in
#                     schema names / $refs). Off by default.
#       --paths FILE  External-projects list file (default: scripts/compare-spec.paths).
#       --no-testdata Do not auto-include projects under testdata/.
#       --bin PATH    Use an existing apispec binary instead of building one.
#   -h, --help        Show this help.
#
# Exit status: non-zero if any path has drift (a response status gained/lost, a
# dropped key, or an in-place value change), a missing snapshot, or a generation
# failure.
#
set -euo pipefail


REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GEN_NAME=".compare_gen.yaml"   # temp filename written inside each path

VERSION=""
KEEP=0
SHOW_ALL=0
STRICT=0
SELF_TEST=0
CMP_FILES=()
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
    --self-test)    SELF_TEST=1; shift ;;
    --compare-files) CMP_FILES=("$2" "$3"); shift 3 ;;
    --paths)        PATHS_FILE="$2"; shift 2 ;;
    --no-testdata)  NO_TESTDATA=1; shift ;;
    --bin)          APISPEC_BIN="$2"; shift 2 ;;
    -h|--help)      usage; exit 0 ;;
    -*)             echo "Unknown option: $1" >&2; usage; exit 2 ;;
    *)              PATHS+=("$1"); shift ;;
  esac
done

# self_test checks the comparator's KEY SEMANTICS -- which differences it treats
# as drift, and which as the same thing said differently -- against built-in
# cases. See scripts/compare-spec-selftest.py for what each case pins and why.
self_test() {
  python3 "$REPO_ROOT/scripts/compare-spec-selftest.py" "$REPO_ROOT"
}

compare_py() {
python3 - "$1" "$2" "$SHOW_ALL" "$STRICT" <<'PY'
import json, sys, yaml

ref_file, gen_file = sys.argv[1], sys.argv[2]
show_added = sys.argv[3] == "1"
strict     = sys.argv[4] == "1"

HTTP_METHODS = {"get", "put", "post", "delete", "options", "head", "patch", "trace"}

# Lists whose meaning is membership, not order, and which are therefore keyed by
# VALUE rather than by index. Sorting (below) already stops a reorder looking like
# a change, but it does not help an INSERTION: keyed positionally, adding one
# value to a 32-value enum reports every later value as CHANGED, which both
# invents drift and buries whether anything was genuinely dropped.
#
# `required` and `tags` have the same set-like shape; add them here if their
# insert-cascade becomes noisy too. They are left out for now because only enum
# has actually produced false drift in practice.
SET_VALUED_KEYS = {"enum"}

# Lists of OBJECTS whose members have a stable identity, keyed by that identity
# rather than by index for the same reason. A parameter is identified by
# (name, in) per the OpenAPI spec, so inserting a header parameter ahead of a
# path parameter should report one added parameter — not rename every one after
# it. Falls back to positional keying when the identity is not usable (a $ref'd
# parameter has no name, and duplicates would collide), so the comparison is
# never made less precise than it was.
IDENTITY_KEYED_LISTS = {"parameters": ("name", "in")}

def member_token(v):
    # Stable, type-aware identity for a whole set member. json rather than repr:
    # it gives object and array members a canonical form (sorted keys), which
    # repr does not, and it still keeps 1, 1.0, "1", true and null apart.
    return json.dumps(v, sort_keys=True, default=str)

def identity_tokens(obj, fields):
    # Tokens for an identity-keyed list, or None when they cannot be trusted.
    if not all(isinstance(v, dict) for v in obj):
        return None
    tokens = []
    for v in obj:
        if not all(v.get(f) is not None for f in fields):
            return None  # a $ref'd or malformed member has no identity
        tokens.append(",".join(str(v[f]) for f in fields))
    if len(set(tokens)) != len(tokens):
        return None  # duplicates would collapse; keep them positional instead
    return tokens

def flatten(obj, prefix=()):
    out = {}
    if isinstance(obj, dict):
        for k, v in obj.items():
            out.update(flatten(v, prefix + (str(k),)))
    elif isinstance(obj, list):
        parent = prefix[-1] if prefix else None
        if obj and parent in SET_VALUED_KEYS:
            # A member is identified by WHAT IT IS, so one added value is one
            # ADDED entry and one removed value is one MISSING entry. Whole
            # members, including object and array ones, which OpenAPI allows.
            for v in obj:
                out[prefix + (f"[={member_token(v)}]",)] = v
            return out
        if obj and parent in IDENTITY_KEYED_LISTS:
            tokens = identity_tokens(obj, IDENTITY_KEYED_LISTS[parent])
            if tokens is not None:
                for token, v in zip(tokens, obj):
                    out.update(flatten(v, prefix + (f"[{token}]",)))
                return out
        if obj and all(not isinstance(v, (dict, list)) for v in obj):
            # Sort the remaining all-scalar lists (required, tags, security
            # scopes, ...) so a cosmetic reorder does not masquerade as CHANGED.
            obj = sorted(obj, key=lambda x: (str(type(x)), str(x)))
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
    if isinstance(v, list):
        return [canon_val(x) for x in v]
    if isinstance(v, dict):
        return {k: canon_val(x) for k, x in v.items()}
    return v

def canon_entry(key, value):
    # Canonicalize key and value TOGETHER. A set member's key is derived from its
    # value, so canonicalizing only the value would leave two spellings of the
    # same $ref keyed differently and report one MISSING plus one ADDED where
    # there is no drift at all.
    cvalue = canon_val(value)
    if key and key[-1].startswith("[=") and cvalue != value:
        key = key[:-1] + (f"[={member_token(cvalue)}]",)
    return canon_key(key), cvalue

def show(key):
    # Human-readable path for display.
    out = ""
    for seg in key:
        if seg.startswith("["):
            out += seg
        else:
            out += ("." if out else "") + seg
    return out

def op_statuses(doc):
    # {(path, METHOD): set(response status keys)} for every operation.
    out = {}
    for p, item in ((doc or {}).get("paths", {}) or {}).items():
        if not isinstance(item, dict):
            continue
        for m, op in item.items():
            if m.lower() not in HTTP_METHODS or not isinstance(op, dict):
                continue
            resps = op.get("responses", {}) or {}
            out[(p, m.upper())] = {str(k) for k in resps}
    return out

with open(ref_file) as f: ref_doc = yaml.safe_load(f) or {}
with open(gen_file) as f: gen_doc = yaml.safe_load(f) or {}

# Say which file is not a spec, rather than failing inside the walk with an
# AttributeError. Reachable by hand now that --compare-files takes any two paths.
for label, path, doc in (("snapshot", ref_file, ref_doc), ("generated", gen_file, gen_doc)):
    if not isinstance(doc, dict):
        print(f"  error: {label} {path} is not an OpenAPI document "
              f"(parsed as {type(doc).__name__})")
        sys.exit(2)

ref_raw = flatten(ref_doc)
gen_raw = flatten(gen_doc)

if strict:
    ref, gen = ref_raw, gen_raw
else:
    ref = dict(canon_entry(k, v) for k, v in ref_raw.items())
    gen = dict(canon_entry(k, v) for k, v in gen_raw.items())

missing = sorted((k for k in ref if k not in gen), key=show)
changed = sorted((k for k in ref if k in gen and ref[k] != gen[k]), key=show)
added   = sorted((k for k in gen if k not in ref), key=show)

# Per-operation response-status-set diffs (order-independent).
ref_st, gen_st = op_statuses(ref_doc), op_statuses(gen_doc)
status_diffs = []
for opkey in sorted(set(ref_st) | set(gen_st)):
    lost = sorted(ref_st.get(opkey, set()) - gen_st.get(opkey, set()))
    gained = sorted(gen_st.get(opkey, set()) - ref_st.get(opkey, set()))
    if lost or gained:
        status_diffs.append((opkey, lost, gained))

# Note the schema-rename normalization if it actually merged anything.
if not strict:
    renamed = sum(1 for k in ref_raw if canon_key(k) != k)
    if renamed:
        print(f"  (note: schema names canonicalized '.'<->'_'; {renamed} keys "
              f"normalized — pass --strict to compare literally)")

if status_diffs:
    print(f"  STATUS CHANGES ({len(status_diffs)}) — response status set differs:")
    for (p, m), lost, gained in status_diffs:
        parts = []
        if lost:   parts.append("lost "   + ",".join(lost))
        if gained: parts.append("gained " + ",".join(gained))
        print(f"    ! {m} {p}: {'; '.join(parts)}")
else:
    print("  STATUS CHANGES (0) — response status sets unchanged.")

if missing:
    print(f"  MISSING ({len(missing)}) — in snapshot, absent from generated:")
    for k in missing:
        print(f"    - {show(k)} = {ref[k]!r}")
else:
    print("  MISSING (0) — nothing from the snapshot was dropped.")

if changed:
    print(f"  CHANGED ({len(changed)}) — same key, different value:")
    for k in changed:
        print(f"    ~ {show(k)}: {ref[k]!r} -> {gen[k]!r}")
else:
    print("  CHANGED (0) — no in-place value changes.")

if show_added and added:
    print(f"  ADDED ({len(added)}) — new in generated:")
    for k in added:
        print(f"    + {show(k)} = {gen[k]!r}")

# Fail on any drift that removes or alters documented behaviour: a status
# gained/lost, a dropped key, or an in-place value change.
sys.exit(1 if (status_diffs or missing or changed) else 0)
PY
}

# Early-exit modes, dispatched here because they need the comparator above and
# nothing below: neither resolves a project set, so neither should be made to
# discover one first.
if [[ $SELF_TEST -eq 1 ]]; then
  self_test; exit $?
fi
if [[ ${#CMP_FILES[@]} -gt 0 ]]; then
  compare_py "${CMP_FILES[0]}" "${CMP_FILES[1]}"; exit $?
fi

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
# Reassign safely: expanding an empty array under `set -u` is an error in
# older bash (e.g. macOS 3.2), which crashed when every path was skipped.
PATHS=()
[[ ${#VALID[@]} -gt 0 ]] && PATHS=("${VALID[@]}")
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

# Build apispec once unless a binary was supplied. Remove the self-built binary
# on exit so it doesn't accumulate in $TMPDIR (a supplied --bin is left alone).
if [[ -z "$APISPEC_BIN" ]]; then
  APISPEC_BIN="$(mktemp -t apispec.XXXXXX)"
  trap 'rm -f "$APISPEC_BIN"' EXIT
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
# and reports differences. Schema component names and $ref targets are
# canonicalized ('.' <-> '_') so a cosmetic rename of the sanitizer does not
# masquerade as hundreds of drops.
#
# It surfaces — and FAILS on — three kinds of DRIFT so no change slips through
# silently (a regression MISSING-only mode used to hide):
#   * STATUS CHANGES — per operation, the set of response status codes gained or
#     lost a status (order-independent). This catches e.g. a route degrading to
#     `default` because a status could no longer be resolved.
#   * MISSING — a key present in the snapshot is absent from the generated spec.
#   * CHANGED — a key present in both has a different value (a $ref retargeted, a
#     schema type flipped in place, a `required`/format changed).
# Enum values are compared as SETS (keyed by value, not position), so adding or
# removing one reports exactly that one value rather than a cascade of CHANGED
# entries for everything after it.
# ADDED keys (new routes/fields) are informational and shown only with --all,
# except added statuses, which the STATUS section always reports and fails on.

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
  echo "RESULT: no drift across all paths (status sets, keys, and values match)."
else
  echo "RESULT: drift found — status changes, missing/changed keys, and/or missing snapshots (see above)."
fi
exit $OVERALL
