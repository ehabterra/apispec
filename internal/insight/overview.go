// Package insight derives an API-centric analysis ("insight") from a
// generated OpenAPI spec plus the analysis metadata. It is intentionally
// read-only and side-effect free so it can be unit-tested with
// hand-built inputs and reused by the apispecui server.
//
// Phase 3 covers the whole-API Overview: route/method/status/content
// histograms, a resolution-health score, a navigable "needs attention"
// issue list, component/type stats, and call-graph stats. Per-endpoint
// tracker-tree metrics (call fan-out, parameter propagation depth, …)
// arrive in Phase 4.
package insight

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
	"github.com/ehabterra/apispec/internal/spec"
)

const (
	refPrefix         = "#/components/schemas/"
	placeholderMarker = "External or unresolved type"
)

// Count is a labelled tally used for the histograms.
type Count struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Issue is one "needs attention" finding tied to a route.
type Issue struct {
	Severity string `json:"severity"` // "warn" | "info"
	Kind     string `json:"kind"`     // unresolved-type | dangling-ref | no-responses | missing-body | wrapper-specialised
	Method   string `json:"method"`
	Path     string `json:"path"`
	Detail   string `json:"detail"`
	Ref      string `json:"ref,omitempty"` // component name involved, if any
}

// Health is the resolution-health summary.
type Health struct {
	Score       int `json:"score"` // 0..100
	TotalRoutes int `json:"totalRoutes"`
	CleanRoutes int `json:"cleanRoutes"`
}

// CallGraphStats summarises the analysed call graph.
type CallGraphStats struct {
	Packages     int     `json:"packages"`
	Functions    int     `json:"functions"`
	Edges        int     `json:"edges"`
	EdgeKinds    []Count `json:"edgeKinds"`    // composition: project / library / standard
	HotFunctions []Count `json:"hotFunctions"` // most-called project functions (fan-in)
	BusyPackages []Count `json:"busyPackages"` // project packages with the most functions
}

// RouteRef is a lightweight route pointer for the endpoint picker.
type RouteRef struct {
	Method string   `json:"method"`
	Path   string   `json:"path"`
	Tags   []string `json:"tags"`
}

// SecurityStats summarises how authentication is applied across the API.
type SecurityStats struct {
	SchemesDefined int      `json:"schemesDefined"` // count of components.securitySchemes
	Schemes        []string `json:"schemes"`        // their names (sorted)
	GlobalSecurity bool     `json:"globalSecurity"` // a document-level security requirement is set
	Protected      int      `json:"protected"`      // operations that require auth (explicit or inherited)
	Public         int      `json:"public"`         // operations explicitly opted out (security: [])
	Unsecured      int      `json:"unsecured"`      // operations with no security at all (no auth)
	BySchemeUsage  []Count  `json:"bySchemeUsage"`  // operations requiring each scheme
}

// OverviewReport is the whole-API insight payload.
type OverviewReport struct {
	Routes        int            `json:"routes"`     // distinct path templates
	Operations    int            `json:"operations"` // method+path combinations
	Endpoints     []RouteRef     `json:"endpoints"`  // for the endpoint picker
	ByMethod      []Count        `json:"byMethod"`
	ByTag         []Count        `json:"byTag"`
	ByStatus      []Count        `json:"byStatus"`
	ByContentType []Count        `json:"byContentType"`
	Components    int            `json:"components"`
	TopTypes      []Count        `json:"topTypes"`
	Health        Health         `json:"health"`
	Security      SecurityStats  `json:"security"`
	Issues        []Issue        `json:"issues"`
	CallGraph     CallGraphStats `json:"callGraph"`
}

// operationsOf returns the (method, *Operation) pairs declared on a path.
func operationsOf(pi spec.PathItem) []struct {
	Method string
	Op     *spec.Operation
} {
	out := []struct {
		Method string
		Op     *spec.Operation
	}{}
	add := func(m string, op *spec.Operation) {
		if op != nil {
			out = append(out, struct {
				Method string
				Op     *spec.Operation
			}{m, op})
		}
	}
	add("GET", pi.Get)
	add("POST", pi.Post)
	add("PUT", pi.Put)
	add("DELETE", pi.Delete)
	add("PATCH", pi.Patch)
	add("OPTIONS", pi.Options)
	add("HEAD", pi.Head)
	return out
}

// BuildOverview projects a generated spec + metadata into an
// OverviewReport. Both inputs may be nil (returns a zero-value report).
func BuildOverview(s *spec.OpenAPISpec, meta *metadata.Metadata) *OverviewReport {
	rep := &OverviewReport{}
	if s == nil {
		return rep
	}

	componentNames := map[string]*spec.Schema{}
	if s.Components != nil {
		for name, sc := range s.Components.Schemas {
			componentNames[name] = sc
		}
	}
	rep.Components = len(componentNames)

	methodC := map[string]int{}
	tagC := map[string]int{}
	statusC := map[string]int{}
	ctC := map[string]int{}
	typeC := map[string]int{}

	clean := 0
	total := 0
	rep.Endpoints = []RouteRef{}

	// Stable path order for deterministic output.
	paths := make([]string, 0, len(s.Paths))
	for p := range s.Paths {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, p := range paths {
		rep.Routes++
		for _, mo := range operationsOf(s.Paths[p]) {
			total++
			rep.Operations++
			methodC[mo.Method]++
			for _, t := range mo.Op.Tags {
				tagC[t]++
			}
			rep.Endpoints = append(rep.Endpoints, RouteRef{Method: mo.Method, Path: p, Tags: mo.Op.Tags})

			routeIssues := collectOperationIssues(mo.Method, p, mo.Op, componentNames, statusC, ctC, typeC)
			rep.Issues = append(rep.Issues, routeIssues...)
			if !hasWarn(routeIssues) {
				clean++
			}
		}
	}

	rep.ByMethod = sortedCounts(methodC, true)
	rep.ByTag = sortedCounts(tagC, true)
	rep.ByStatus = sortedCounts(statusC, false) // status: by name (numeric-ish) asc
	rep.ByContentType = sortedCounts(ctC, true)
	rep.TopTypes = topN(sortedCounts(typeC, true), 10)

	rep.Health = Health{TotalRoutes: total, CleanRoutes: clean}
	if total > 0 {
		rep.Health.Score = int((float64(clean) / float64(total) * 100) + 0.5)
	} else {
		rep.Health.Score = 100
	}

	// Stable issue order: warns first, then by path/method.
	sort.SliceStable(rep.Issues, func(i, j int) bool {
		a, b := rep.Issues[i], rep.Issues[j]
		if (a.Severity == "warn") != (b.Severity == "warn") {
			return a.Severity == "warn"
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Method < b.Method
	})

	// Never return nil slices: Go marshals nil as JSON null, which the
	// UI would choke on (rep.issues.filter is not a function on null).
	if rep.Issues == nil {
		rep.Issues = []Issue{}
	}

	rep.Security = securityStats(s)
	rep.CallGraph = callGraphStats(meta)
	return rep
}

// securityStats classifies every operation as protected / public / unsecured,
// honouring OpenAPI inheritance: an operation with no `security` field inherits
// the document-level requirement, an explicit empty `security: []` opts out
// (public), and a non-empty requirement (own or inherited) means protected.
func securityStats(s *spec.OpenAPISpec) SecurityStats {
	st := SecurityStats{Schemes: []string{}, BySchemeUsage: []Count{}}
	if s == nil {
		return st
	}
	if s.Components != nil {
		for name := range s.Components.SecuritySchemes {
			st.Schemes = append(st.Schemes, name)
		}
		sort.Strings(st.Schemes)
		st.SchemesDefined = len(st.Schemes)
	}
	st.GlobalSecurity = len(s.Security) > 0

	usage := map[string]int{}
	for _, p := range s.Paths {
		for _, mo := range operationsOf(p) {
			// Resolve the effective requirement for this operation.
			var eff []spec.SecurityRequirement
			if mo.Op.Security != nil {
				if len(*mo.Op.Security) == 0 {
					st.Public++ // explicit security: []
					continue
				}
				eff = *mo.Op.Security
			} else {
				eff = s.Security // inherit document-level
			}
			if len(eff) == 0 {
				st.Unsecured++
				continue
			}
			st.Protected++
			seen := map[string]bool{}
			for _, req := range eff {
				for name := range req {
					if !seen[name] {
						seen[name] = true
						usage[name]++
					}
				}
			}
		}
	}
	st.BySchemeUsage = sortedCounts(usage, true)
	return st
}

func collectOperationIssues(method, path string, op *spec.Operation, comps map[string]*spec.Schema, statusC, ctC, typeC map[string]int) []Issue {
	var issues []Issue

	if len(op.Responses) == 0 {
		issues = append(issues, Issue{
			Severity: "warn", Kind: "no-responses", Method: method, Path: path,
			Detail: "no responses detected for this operation",
		})
	}

	// request body
	if op.RequestBody != nil {
		for ct, mt := range op.RequestBody.Content {
			ctC[ct]++
			issues = append(issues, refIssues(method, path, "request body", mt.Schema, comps, typeC)...)
		}
	}

	// responses
	for status, resp := range op.Responses {
		statusC[status]++
		for ct, mt := range resp.Content {
			ctC[ct]++
			issues = append(issues, refIssues(method, path, "response "+status, mt.Schema, comps, typeC)...)
			if isWrapperSpecialised(mt.Schema) {
				issues = append(issues, Issue{
					Severity: "info", Kind: "wrapper-specialised", Method: method, Path: path,
					Detail: "response " + status + " is a wrapper/envelope with a specialised data payload",
				})
			}
		}
	}

	// parameter schemas can also reference components
	for _, prm := range op.Parameters {
		issues = append(issues, refIssues(method, path, "parameter "+prm.Name, prm.Schema, comps, typeC)...)
	}
	return issues
}

// refIssues records every component ref reachable from a schema, tallies
// type usage, and flags refs that dangle or resolve to a placeholder.
func refIssues(method, path, where string, sc *spec.Schema, comps map[string]*spec.Schema, typeC map[string]int) []Issue {
	var issues []Issue
	for _, name := range schemaRefs(sc) {
		typeC[name]++
		target, ok := comps[name]
		if !ok {
			issues = append(issues, Issue{
				Severity: "warn", Kind: "dangling-ref", Method: method, Path: path, Ref: name,
				Detail: where + " references a schema that has no component definition",
			})
			continue
		}
		if target != nil && strings.Contains(target.Description, placeholderMarker) {
			issues = append(issues, Issue{
				Severity: "warn", Kind: "unresolved-type", Method: method, Path: path, Ref: name,
				Detail: where + " resolves to an unresolved/external placeholder type",
			})
		}
	}
	return issues
}

// schemaRefs collects every "#/components/schemas/NAME" target reachable
// from a schema (deduped per schema tree).
func schemaRefs(sc *spec.Schema) []string {
	seen := map[string]bool{}
	var out []string
	var walk func(s *spec.Schema)
	walk = func(s *spec.Schema) {
		if s == nil {
			return
		}
		if strings.HasPrefix(s.Ref, refPrefix) {
			name := strings.TrimPrefix(s.Ref, refPrefix)
			if !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		}
		walk(s.Items)
		walk(s.AdditionalProperties)
		walk(s.Not)
		for _, c := range s.Properties {
			walk(c)
		}
		for _, c := range s.AllOf {
			walk(c)
		}
		for _, c := range s.OneOf {
			walk(c)
		}
		for _, c := range s.AnyOf {
			walk(c)
		}
	}
	walk(sc)
	return out
}

// isWrapperSpecialised detects the allOf[base $ref, {object with props}]
// shape the wrapper-response specialiser emits.
func isWrapperSpecialised(sc *spec.Schema) bool {
	if sc == nil || len(sc.AllOf) < 2 {
		return false
	}
	hasRef, hasInline := false, false
	for _, part := range sc.AllOf {
		if part == nil {
			continue
		}
		if strings.HasPrefix(part.Ref, refPrefix) {
			hasRef = true
		}
		if part.Ref == "" && len(part.Properties) > 0 {
			hasInline = true
		}
	}
	return hasRef && hasInline
}

func callGraphStats(meta *metadata.Metadata) CallGraphStats {
	st := CallGraphStats{EdgeKinds: []Count{}, HotFunctions: []Count{}, BusyPackages: []Count{}}
	if meta == nil {
		return st
	}
	sp := meta.StringPool
	st.Packages = len(meta.Packages)
	st.Edges = len(meta.CallGraph)

	// Functions per project package (and the total).
	busy := map[string]int{}
	for path, pkg := range meta.Packages {
		cnt := 0
		for _, f := range pkg.Files {
			cnt += len(f.Functions)
			for _, t := range f.Types {
				cnt += len(t.Methods)
			}
		}
		for _, t := range pkg.Types {
			cnt += len(t.Methods)
		}
		st.Functions += cnt
		if cnt > 0 && classifyPkg(meta, path, "") == "project" {
			busy[lastSegment(path)] += cnt
		}
	}
	st.BusyPackages = topN(sortedCounts(busy, true), 6)

	// Edge composition + project fan-in (how many times each project
	// function is called). Needs the string pool to resolve callees.
	if sp != nil {
		kinds := map[string]int{}
		fanIn := map[string]int{}
		for i := range meta.CallGraph {
			e := &meta.CallGraph[i]
			name := sp.GetString(e.Callee.Name)
			kind := classifyPkg(meta, sp.GetString(e.Callee.Pkg), sp.GetString(e.Callee.RecvType))
			if builtinFuncs[name] {
				kind = "standard"
			}
			kinds[kind]++
			if kind == "project" {
				fanIn[shortLabel(e.Callee.BaseID())]++
			}
		}
		for _, k := range []string{"project", "library", "standard"} {
			if kinds[k] > 0 {
				st.EdgeKinds = append(st.EdgeKinds, Count{Name: k, Count: kinds[k]})
			}
		}
		st.HotFunctions = topN(sortedCounts(fanIn, true), 6)
	}
	return st
}

// lastSegment returns the final "/"-separated segment of an import path.
func lastSegment(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

func hasWarn(issues []Issue) bool {
	for _, i := range issues {
		if i.Severity == "warn" {
			return true
		}
	}
	return false
}

// sortedCounts converts a tally map to a slice. byCount=true sorts by
// count desc (ties alpha); byCount=false sorts by name asc.
func sortedCounts(m map[string]int, byCount bool) []Count {
	out := make([]Count, 0, len(m))
	for k, v := range m {
		out = append(out, Count{Name: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if byCount && out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func topN(c []Count, n int) []Count {
	if len(c) > n {
		return c[:n]
	}
	return c
}

// Summary is a one-line human description, handy for logs/tests.
func (r *OverviewReport) Summary() string {
	warns := 0
	for _, i := range r.Issues {
		if i.Severity == "warn" {
			warns++
		}
	}
	return fmt.Sprintf("%d routes, %d ops, health %d%%, %d warnings",
		r.Routes, r.Operations, r.Health.Score, warns)
}
