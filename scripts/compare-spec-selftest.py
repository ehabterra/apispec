#!/usr/bin/env python3
# Copyright 2026 Ehab Terra
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Self-test for compare-spec.sh's KEY SEMANTICS.

The comparator's job is to tell drift from the same thing said differently, and
that judgement lives entirely in how it keys values. Keyed by position, a list
reports an INSERTION as a change to every member after it — which both invents
drift and buries whether anything was genuinely dropped, the one thing the tool
exists to answer.

Each case writes a pair of tiny specs, runs the real comparator over them, and
asserts the reported counts and the exit status. A regression then surfaces as a
named failing case here, instead of as noise in a real project comparison months
later.

Run via: scripts/compare-spec.sh --self-test
"""

import json
import os
import subprocess
import sys
import tempfile


def spec(schema=None, params=None):
    doc = {"openapi": "3.1.1", "info": {"title": "t", "version": "1"}, "paths": {}}
    if schema is not None:
        doc["components"] = {"schemas": {"S": schema}}
    if params is not None:
        doc["paths"] = {
            "/x": {"get": {"parameters": params, "responses": {"200": {"description": "OK"}}}}
        }
    return doc


def run(script, ref, gen, flags):
    with tempfile.TemporaryDirectory() as d:
        a, b = os.path.join(d, "ref.yaml"), os.path.join(d, "gen.yaml")
        for path, doc in ((a, ref), (b, gen)):
            with open(path, "w") as f:
                json.dump(doc, f)  # JSON is valid YAML
        p = subprocess.run(
            ["bash", script, "--compare-files", a, b, *flags],
            capture_output=True,
            text=True,
        )
        return p.returncode, p.stdout + p.stderr


def count(out, label):
    for line in out.splitlines():
        line = line.strip()
        if line.startswith(label + " ("):
            return int(line.split("(", 1)[1].split(")", 1)[0])
    return -1


PATH_ID = {"name": "id", "in": "path", "required": True, "schema": {"type": "string"}}
HEADER = {"name": "Retry-After", "in": "header", "schema": {"type": "string"}}
REF_PARAM = {"$ref": "#/components/parameters/Foo"}

ENUM32 = ["v%02d" % i for i in range(28)] + [
    "unenrolled",
    "user_requested",
    "within_maximum",
    "within_target",
]
ENUM35 = sorted(ENUM32 + ["teacher_extended", "teacher_restored", "teacher_withdrew"])


def enum_spec(values):
    return spec({"properties": {"f": {"enum": values}}})


# (name, ref, gen, missing, changed, added, should_fail, flags)
CASES = [
    # The case this mechanism exists for: three values added to a 32-value enum
    # reported four CHANGED and failed, when nothing changed and nothing was lost.
    ("enum: values inserted", enum_spec(sorted(ENUM32)), enum_spec(ENUM35), 0, 0, 3, False, ["--all"]),
    ("enum: value dropped still fails", enum_spec(["a", "b", "c"]), enum_spec(["a", "c"]), 1, 0, 0, True, []),
    ("enum: mixed types not conflated", enum_spec([1, "1", True, None]), enum_spec([1, "1", True, None]), 0, 0, 0, False, []),
    ("enum: int replaced by string is drift", enum_spec([1, 2]), enum_spec(["1", 2]), 1, 0, 1, True, ["--all"]),
    # OpenAPI allows object and array enum members, so they are set members too
    # rather than positional paths that cascade on insertion.
    ("enum: object members inserted", enum_spec([{"k": 1}, {"k": 3}]), enum_spec([{"k": 1}, {"k": 2}, {"k": 3}]), 0, 0, 1, False, ["--all"]),
    ("enum: object member dropped still fails", enum_spec([{"k": 1}, {"k": 2}]), enum_spec([{"k": 1}]), 1, 0, 0, True, []),
    # A schema name spelled '.' on one side and '_' on the other canonicalizes to
    # the same value, so the key derived from that value has to canonicalize too.
    ("enum: $ref spelling canonicalizes in key and value",
     enum_spec(["#/components/schemas/Foo.Bar"]), enum_spec(["#/components/schemas/Foo_Bar"]), 0, 0, 0, False, []),
    # Parameters are identified by (name, in): inserting one must not rename the rest.
    ("parameters: inserted ahead of an existing one", spec(params=[PATH_ID]), spec(params=[HEADER, PATH_ID]), 0, 0, 3, False, ["--all"]),
    # Four leaves go with it: name, in, required and schema.type.
    ("parameters: dropped still fails", spec(params=[HEADER, PATH_ID]), spec(params=[HEADER]), 4, 0, 0, True, []),
    ("parameters: a changed attribute is still reported", spec(params=[PATH_ID]), spec(params=[{**PATH_ID, "required": False}]), 0, 1, 0, True, []),
    # Without a usable identity the list stays positional, so precision never
    # drops below what it was before identity keying.
    ("parameters: $ref members fall back to positional", spec(params=[REF_PARAM]), spec(params=[REF_PARAM]), 0, 0, 0, False, []),
]


def main():
    repo = sys.argv[1] if len(sys.argv) > 1 else os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    script = os.path.join(repo, "scripts", "compare-spec.sh")

    failures = 0
    for name, ref, gen, want_missing, want_changed, want_added, want_fail, flags in CASES:
        code, out = run(script, ref, gen, flags)
        got_added = count(out, "ADDED") if "--all" in flags else want_added
        got = (count(out, "MISSING"), count(out, "CHANGED"), got_added)
        want = (want_missing, want_changed, want_added)
        if got == want and bool(code) == want_fail:
            print(f"  ok   {name}")
            continue
        failures += 1
        print(f"  FAIL {name}")
        print(f"       want missing/changed/added={want} fails={want_fail}")
        print(f"       got  missing/changed/added={got} fails={bool(code)}")
        for line in out.splitlines():
            print("       | " + line)

    print()
    if failures:
        print(f"self-test: {failures} of {len(CASES)} cases FAILED")
        return 1
    print(f"self-test: all {len(CASES)} cases passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
