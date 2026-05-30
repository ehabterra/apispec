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
	b.WriteString(fmt.Sprintf("%s\n\n", rep.Summary()))

	if len(warns) == 0 {
		b.WriteString("No blocking issues were detected — the spec resolved cleanly. ")
		b.WriteString("You can still ask for review suggestions on the config below.\n\n")
	} else {
		b.WriteString(fmt.Sprintf("There are %d issue(s). For **each**, suggest the *smallest* fix and give it verbatim — either:\n", len(warns)))
		b.WriteString("- (A) a Go code change in the handler/type, or\n")
		b.WriteString("- (B) an apispec config entry (`externalTypes` / `typeMapping` / `overrides`) to add.\n")
		b.WriteString("State which you chose and why.\n\n")

		for idx, is := range warns {
			b.WriteString(fmt.Sprintf("## %d. %s %s\n", idx+1, is.Method, red(is.Path)))
			b.WriteString(fmt.Sprintf("- **Kind:** %s\n", is.Kind))
			b.WriteString(fmt.Sprintf("- **Problem:** %s\n", red(is.Detail)))
			if is.Ref != "" {
				b.WriteString(fmt.Sprintf("- **Schema/component:** `%s`\n", red(is.Ref)))
			}
			b.WriteString("\n")
		}
	}

	if strings.TrimSpace(opts.ConfigYAML) != "" {
		b.WriteString("## Current apispec config\n\n```yaml\n")
		b.WriteString(red(strings.TrimRight(opts.ConfigYAML, "\n")))
		b.WriteString("\n```\n")
	}

	return b.String()
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
	if len(warns) > 0 {
		b.WriteString("## Problem(s)\n")
		for _, is := range warns {
			fmt.Fprintf(&b, "- **%s** — %s", is.Kind, red(is.Detail))
			if is.Ref != "" {
				fmt.Fprintf(&b, " (`%s`)", red(is.Ref))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// scoped trace
	if len(rep.Trace.Edges) > 0 {
		b.WriteString("## Resolution trace (call subtree, this endpoint)\n```\n")
		max := 40
		for i, e := range rep.Trace.Edges {
			if i >= max {
				b.WriteString("…\n")
				break
			}
			fmt.Fprintf(&b, "%s → %s\n", red(shortLabel(e.Source)), red(shortLabel(e.Target)))
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
	b.WriteString("Suggest the smallest fix for the problem(s) above — either a Go code change ")
	b.WriteString("in the handler/types, or an apispec config entry (`externalTypes` / `typeMapping` / `overrides`). ")
	b.WriteString("Give it verbatim and say which you chose.\n")
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
	defer f.Close()

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
