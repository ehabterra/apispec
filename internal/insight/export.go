// Copyright 2026 Ehab Terra
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package insight

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ExportOptions controls the AI prompt bundle.
type ExportOptions struct {
	ConfigYAML string // current effective config, included so the AI can suggest config fixes
	ModulePath string // used for redaction
	Redact     bool   // replace the module path with a placeholder
}

// BuildExportMarkdown turns the open issues into a self-contained,
// fix-ready prompt for an AI assistant. The same data that explains an
// issue is what makes it exportable: route, problem, the component
// involved, and the current config (so the AI can propose a config fix
// — externalTypes / typeMapping / overrides — not just a code change).
//
// Phase 3 is the whole-API bundle (every open issue). Per-endpoint
// source snippets and the resolution trace are added in Phase 4.
func BuildExportMarkdown(rep *OverviewReport, opts ExportOptions) string {
	red := func(s string) string {
		if opts.Redact && opts.ModulePath != "" {
			return strings.ReplaceAll(s, opts.ModulePath, "example.com/app")
		}
		return s
	}

	var warns []Issue
	for _, i := range rep.Issues {
		if i.Severity == "warn" {
			warns = append(warns, i)
		}
	}

	var b strings.Builder
	b.WriteString("# apispec — issues to resolve\n\n")
	fmt.Fprintf(&b, "%s\n\n", rep.Summary())

	// Nothing to fix → don't emit an AI prompt or dump the config. A clean
	// spec needs no action, so the export is just a one-line clean bill.
	if len(warns) == 0 {
		b.WriteString("No blocking issues were detected — the spec resolved cleanly. Nothing to fix.\n")
		return b.String()
	}

	modPrefix := moduleRefPrefix(opts.ModulePath)
	inModule := func(ref string) bool {
		return modPrefix != "" && strings.HasPrefix(ref, modPrefix)
	}

	fmt.Fprintf(&b, "There are %d issue(s). Suggest the *smallest* fix for **each** — but first classify the type, because the right fix differs:\n\n", len(warns))
	b.WriteString("- **In-module type** (defined in your own module): apispec should resolve it automatically, so a placeholder almost always means the analyzer never saw the definition — prefer **(A) a Go code / analysis fix**. Common causes: the package failed to type-check and was skipped (re-run with `-verbose` and look for \"Skipping package … due to errors\"), the type uses generics/embedding/aliases the resolver didn't follow, or the body/response isn't assigned in a traceable way. Add a config entry only as a last resort — it hand-writes a schema apispec should derive, and it rots when the type changes.\n")
	b.WriteString("- **External type** (third-party, outside your module): prefer **(B) an apispec config entry** (`externalTypes` / `typeMapping` / `overrides`) mapping it to an explicit schema — like the existing `uuid.UUID` entry.\n")
	b.WriteString("State the class, which option you chose, and why. Give the fix verbatim.\n\n")

	for idx, is := range warns {
		fmt.Fprintf(&b, "## %d. %s %s\n", idx+1, is.Method, red(is.Path))
		fmt.Fprintf(&b, "- **Kind:** %s\n", is.Kind)
		fmt.Fprintf(&b, "- **Problem:** %s\n", red(is.Detail))
		if is.Ref != "" {
			fmt.Fprintf(&b, "- **Schema/component:** `%s`\n", red(is.Ref))
			if modPrefix != "" {
				if inModule(is.Ref) {
					b.WriteString("- **Type class:** in-module → should auto-resolve; prefer a code/analysis fix (A)\n")
				} else {
					b.WriteString("- **Type class:** external → prefer a config entry (B)\n")
				}
			}
		}
		b.WriteString("\n")
	}

	if strings.TrimSpace(opts.ConfigYAML) != "" {
		b.WriteString("## Current apispec config\n\n```yaml\n")
		b.WriteString(red(strings.TrimRight(opts.ConfigYAML, "\n")))
		b.WriteString("\n```\n")
	}

	return b.String()
}

// moduleRefPrefix sanitizes the module path the same way schema component
// names are built (dots and slashes → underscores), so an in-module schema
// ref can be recognised by a simple prefix check. Returns "" when the module
// path is unknown, in which case callers should skip classification.
func moduleRefPrefix(modulePath string) string {
	if modulePath == "" {
		return ""
	}
	return strings.NewReplacer("/", "_", ".", "_").Replace(modulePath)
}

// BuildEndpointExportMarkdown builds a fix-ready prompt for a single
// endpoint: its request/response/params, issues, the route-scoped call
// trace, a window of the handler's source (pulled from its position),
// and the config — so an AI can propose a code or config fix.
func BuildEndpointExportMarkdown(rep *EndpointReport, opts ExportOptions) string {
	red := func(s string) string {
		if opts.Redact && opts.ModulePath != "" {
			return strings.ReplaceAll(s, opts.ModulePath, "example.com/app")
		}
		return s
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# apispec — endpoint report\n\n")
	fmt.Fprintf(&b, "**%s %s**\n\n", rep.Method, red(rep.Path))
	if rep.Handler != "" {
		fmt.Fprintf(&b, "- Handler: `%s`\n", red(rep.Handler))
	}
	if rep.HandlerPos != "" {
		fmt.Fprintf(&b, "- Source: %s\n", red(rep.HandlerPos))
	}
	b.WriteString("\n")

	// shape
	b.WriteString("## Shape\n")
	if rep.Request != nil {
		fmt.Fprintf(&b, "- Request: %s `%s`\n", rep.Request.ContentType, red(rep.Request.Schema))
	}
	for _, r := range rep.Responses {
		fmt.Fprintf(&b, "- Response %s: %s `%s`\n", r.Status, r.ContentType, red(r.Schema))
	}
	for _, p := range rep.Params {
		fmt.Fprintf(&b, "- Param `%s` in %s: %s%s\n", p.Name, p.In, p.Type, reqStr(p.Required))
	}
	b.WriteString("\n")

	// issues
	var warns []Issue
	for _, i := range rep.Issues {
		if i.Severity == "warn" {
			warns = append(warns, i)
		}
	}
	modPrefix := moduleRefPrefix(opts.ModulePath)
	if len(warns) > 0 {
		b.WriteString("## Problem(s)\n")
		for _, is := range warns {
			fmt.Fprintf(&b, "- **%s** — %s", is.Kind, red(is.Detail))
			if is.Ref != "" {
				fmt.Fprintf(&b, " (`%s`)", red(is.Ref))
				if modPrefix != "" {
					if strings.HasPrefix(is.Ref, modPrefix) {
						b.WriteString(" — _in-module type; should auto-resolve, prefer a code/analysis fix_")
					} else {
						b.WriteString(" — _external type; prefer a config entry_")
					}
				}
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// scoped trace
	if len(rep.Trace.Edges) > 0 {
		// Nodes whose target is a concrete implementation resolved from an
		// interface call — annotated below (with the interface and its
		// implementation count) so the AI sees the interface→impl hop and any
		// ambiguity behind it.
		resolved := make(map[string]TraceNode, len(rep.Trace.Nodes))
		for _, n := range rep.Trace.Nodes {
			if n.Resolved {
				resolved[n.ID] = n
			}
		}
		b.WriteString("## Resolution trace (call subtree, this endpoint)\n")
		b.WriteString("`⟐ impl` marks a concrete implementation resolved from an interface call.\n```\n")
		max := 40
		for i, e := range rep.Trace.Edges {
			if i >= max {
				b.WriteString("…\n")
				break
			}
			marker := ""
			if rn, ok := resolved[e.Target]; ok {
				switch {
				case rn.ResolvedFrom != "" && rn.Alternatives > 1:
					marker = fmt.Sprintf("   ⟐ impl (1 of %d implementations of %s — may be kept general)", rn.Alternatives, rn.ResolvedFrom)
				case rn.ResolvedFrom != "":
					marker = fmt.Sprintf("   ⟐ impl (sole implementation of %s)", rn.ResolvedFrom)
				default:
					marker = "   ⟐ impl (resolved from interface)"
				}
			}
			fmt.Fprintf(&b, "%s → %s%s\n", red(shortLabel(e.Source)), red(shortLabel(e.Target)), marker)
		}
		b.WriteString("```\n\n")
	}

	// handler source window
	if src := readSourceWindow(rep.HandlerPos, 1, 30); src != "" {
		b.WriteString("## Handler source (pulled from your code)\n```go\n")
		b.WriteString(red(src))
		b.WriteString("\n```\n\n")
	}

	if strings.TrimSpace(opts.ConfigYAML) != "" {
		b.WriteString("## Current apispec config\n\n```yaml\n")
		b.WriteString(red(strings.TrimRight(opts.ConfigYAML, "\n")))
		b.WriteString("\n```\n\n")
	}

	b.WriteString("## What I need\n")
	b.WriteString("Suggest the smallest fix for the problem(s) above. For an **in-module type** (your own module) a placeholder usually means the analyzer didn't see the definition — prefer a Go code/analysis fix (check for a skipped package, generics/embedding the resolver missed, or an untraceable assignment). For an **external type** prefer an apispec config entry (`externalTypes` / `typeMapping` / `overrides`). Give the fix verbatim and say which class and option you chose.\n")
	return b.String()
}

func reqStr(req bool) string {
	if req {
		return " (required)"
	}
	return ""
}

// readSourceWindow reads `after` lines starting `before` lines above the
// line in a "file:line[:col]" position. Returns "" on any problem (best
// effort — never fatal).
func readSourceWindow(pos string, before, after int) string {
	file, line := parsePos(pos)
	if file == "" || line <= 0 {
		return ""
	}
	f, err := os.Open(file)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	start := line - before
	if start < 1 {
		start = 1
	}
	end := line + after

	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	n := 0
	for sc.Scan() {
		n++
		if n < start {
			continue
		}
		if n > end {
			break
		}
		out = append(out, sc.Text())
	}
	return strings.Join(out, "\n")
}

// PosFile returns just the file path of a "file:line[:col]" position (empty
// when unparseable). Callers use it to validate the path before reading.
func PosFile(pos string) string {
	f, _ := parsePos(pos)
	return f
}

// SourceSnippet reads a window of source around a file:line position. It
// returns the code, the 1-based line the window starts at, and the target
// line itself (so the UI can number lines and highlight the call site).
// Best-effort: returns ("", 0, 0) on any problem.
func SourceSnippet(pos string, before, after int) (code string, startLine, targetLine int) {
	file, line := parsePos(pos)
	if file == "" || line <= 0 {
		return "", 0, 0
	}
	start := line - before
	if start < 1 {
		start = 1
	}
	return readSourceWindow(pos, before, after), start, line
}

// parsePos splits "file:line:col" (col optional). File paths may contain
// no colons on the platforms we target, so we peel numeric tail fields.
func parsePos(pos string) (string, int) {
	pos = strings.TrimSpace(pos)
	if pos == "" {
		return "", 0
	}
	parts := strings.Split(pos, ":")
	if len(parts) < 2 {
		return "", 0
	}
	// peel a trailing column if present
	last := parts[len(parts)-1]
	if _, err := strconv.Atoi(last); err == nil && len(parts) >= 3 {
		// last is col, second-last is line
		if line, err := strconv.Atoi(parts[len(parts)-2]); err == nil {
			return strings.Join(parts[:len(parts)-2], ":"), line
		}
	}
	if line, err := strconv.Atoi(last); err == nil {
		return strings.Join(parts[:len(parts)-1], ":"), line
	}
	return "", 0
}
