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

// CoverMetric is a have-of-total coverage tally.
type CoverMetric struct {
	Have  int `json:"have"`
	Total int `json:"total"`
}

// Coverage summarises how completely common facets are documented across the
// API. All fields are derived from the generated spec (cheap — no call-graph or
// tracker-tree traversal).
type Coverage struct {
	RequestBody    CoverMetric `json:"requestBody"`    // write ops (POST/PUT/PATCH) that declare a body schema
	ErrorResponses CoverMetric `json:"errorResponses"` // ops that declare at least one 4xx/5xx
	Protected      CoverMetric `json:"protected"`      // ops that require authentication
}

// Resolution splits operations by how completely they resolved: full (no
// issues), partial (works but a detail is missing — a defaulted status or a
// generic body), broken (a dangling ref / unresolved type / no responses
// reaches the output).
type Resolution struct {
	Full    int `json:"full"`
	Partial int `json:"partial"`
	Broken  int `json:"broken"`
}

// InterfaceStats summarises interface→implementation resolution, read directly
// from the metadata's precomputed ImplementedBy index (no tracker-tree build).
type InterfaceStats struct {
	Total         int     `json:"total"`
	SingleImpl    int     `json:"singleImpl"`    // exactly one implementation — unambiguous
	Ambiguous     int     `json:"ambiguous"`     // more than one — may be kept general (erased to any)
	Unimplemented int     `json:"unimplemented"` // no implementation found
	AmbiguousList []Count `json:"ambiguousList"` // ambiguous interface name -> implementation count
}

// VerbDispatch is one handler that serves several HTTP methods from a single
// function via a `switch r.Method` / `if r.Method ==` dispatch (from the
// metadata's precomputed MethodDispatch arms).
type VerbDispatch struct {
	Handler string   `json:"handler"`
	Methods []string `json:"methods"`
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
	Resolution    Resolution     `json:"resolution"`
	Coverage      Coverage       `json:"coverage"`
	Taxonomy      []Count        `json:"taxonomy"` // issue kind -> count (the "why not resolved" breakdown)
	Security      SecurityStats  `json:"security"`
	Issues        []Issue        `json:"issues"`
	Interfaces    InterfaceStats `json:"interfaces"`   // tree insight: interface resolution (cheap read)
	VerbDispatch  []VerbDispatch `json:"verbDispatch"` // tree insight: handlers split by verb (cheap read)
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

			// Resolution state + documentation coverage — both cheap, from the
			// spec we're already walking.
			switch classifyResolution(routeIssues) {
			case "broken":
				rep.Resolution.Broken++
			case "partial":
				rep.Resolution.Partial++
			default:
				rep.Resolution.Full++
			}
			if isWriteMethod(mo.Method) {
				rep.Coverage.RequestBody.Total++
				if hasBodySchema(mo.Op) {
					rep.Coverage.RequestBody.Have++
				}
			}
			rep.Coverage.ErrorResponses.Total++
			if hasErrorResponse(mo.Op) {
				rep.Coverage.ErrorResponses.Have++
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
	// Protected coverage reuses the security classification (cheap).
	rep.Coverage.Protected = CoverMetric{
		Have:  rep.Security.Protected,
		Total: rep.Security.Protected + rep.Security.Public + rep.Security.Unsecured,
	}
	// Taxonomy: the "why not resolved" breakdown — a tally of every issue kind
	// (all severities), so the UI can rank the causes.
	kindC := map[string]int{}
	for _, is := range rep.Issues {
		kindC[is.Kind]++
	}
	rep.Taxonomy = sortedCounts(kindC, true)

	// Tree insights — precomputed metadata reads only (no tracker-tree build,
	// so the Overview stays lightweight).
	rep.Interfaces = interfaceStats(meta)
	rep.VerbDispatch = verbDispatch(meta)

	rep.CallGraph = callGraphStats(meta)
	return rep
}

// classifyResolution maps an operation's issues to a resolution state:
// broken (a dangling ref / unresolved type / no responses reaches the output),
// partial (works but a detail is missing — a generic body or a defaulted
// status), or full (no issues).
func classifyResolution(issues []Issue) string {
	partial := false
	for _, is := range issues {
		switch is.Kind {
		case "dangling-ref", "unresolved-type", "no-responses":
			return "broken"
		case "missing-body", "default-status":
			partial = true
		}
	}
	if partial {
		return "partial"
	}
	return "full"
}

func isWriteMethod(m string) bool {
	switch m {
	case "POST", "PUT", "PATCH":
		return true
	}
	return false
}

// hasBodySchema reports whether an operation declares a request body with a
// schema (any content type).
func hasBodySchema(op *spec.Operation) bool {
	if op.RequestBody == nil {
		return false
	}
	for _, mt := range op.RequestBody.Content {
		if mt.Schema != nil {
			return true
		}
	}
	return false
}

// hasErrorResponse reports whether an operation declares any 4xx or 5xx
// response (a documented failure path), as opposed to only 2xx and/or default.
func hasErrorResponse(op *spec.Operation) bool {
	for status := range op.Responses {
		if len(status) > 0 && (status[0] == '4' || status[0] == '5') {
			return true
		}
	}
	return false
}

// interfaceStats reads the metadata's precomputed ImplementedBy index to
// summarise interface resolution. Cheap: a single pass over declared types, no
// call-graph or tracker traversal.
func interfaceStats(meta *metadata.Metadata) InterfaceStats {
	st := InterfaceStats{AmbiguousList: []Count{}}
	if meta == nil || meta.StringPool == nil {
		return st
	}
	sp := meta.StringPool
	ambig := map[string]int{}
	seen := map[string]bool{} // dedupe types that appear under both file- and package-scope
	visit := func(t *metadata.Type) {
		if sp.GetString(t.Kind) != "interface" {
			return
		}
		key := sp.GetString(t.Pkg) + "." + sp.GetString(t.Name)
		if seen[key] {
			return
		}
		seen[key] = true
		st.Total++
		switch n := len(t.ImplementedBy); n {
		case 0:
			st.Unimplemented++
		case 1:
			st.SingleImpl++
		default:
			st.Ambiguous++
			// Qualify the display label the same way `seen` qualifies its
			// dedupe key: two ambiguous interfaces with the same bare name in
			// different packages (Store, Repository, Handler … are common)
			// would otherwise overwrite each other's count, and — because map
			// iteration order is randomized — which one survived would vary
			// run to run, breaking output determinism.
			label := sp.GetString(t.Name)
			if p := sp.GetString(t.Pkg); p != "" {
				label = lastSegment(p) + "." + label
			}
			ambig[label] = n
		}
	}
	for _, pkg := range meta.Packages {
		for _, f := range pkg.Files {
			for i := range f.Types {
				visit(f.Types[i])
			}
		}
		for i := range pkg.Types {
			visit(pkg.Types[i])
		}
	}
	st.AmbiguousList = topN(sortedCounts(ambig, true), 8)
	return st
}

// verbDispatch reads the metadata's precomputed MethodDispatch arms to list
// handlers that serve several HTTP methods from one function. Cheap: a single
// pass over declared functions.
func verbDispatch(meta *metadata.Metadata) []VerbDispatch {
	out := []VerbDispatch{}
	if meta == nil || meta.StringPool == nil {
		return out
	}
	sp := meta.StringPool
	for _, pkg := range meta.Packages {
		for _, f := range pkg.Files {
			for name, fn := range f.Functions {
				if len(fn.MethodDispatch) == 0 {
					continue
				}
				methods := []string{}
				seen := map[string]bool{}
				for _, b := range fn.MethodDispatch {
					for _, m := range b.Methods {
						if m != "" && !seen[m] {
							seen[m] = true
							methods = append(methods, m)
						}
					}
				}
				if len(methods) < 2 {
					continue // a single-method dispatch isn't a split worth surfacing
				}
				sort.Strings(methods)
				handler := name
				if p := sp.GetString(fn.Pkg); p != "" {
					handler = lastSegment(p) + "." + name
				}
				out = append(out, VerbDispatch{Handler: handler, Methods: methods})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Handler < out[j].Handler })
	return out
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
			// An empty requirement object {} inside the array permits anonymous
			// access (optional auth), so the operation is effectively public —
			// not protected — regardless of any sibling requirements.
			anonymous := false
			for _, req := range eff {
				if len(req) == 0 {
					anonymous = true
					break
				}
			}
			if anonymous {
				st.Public++
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

	// A `default` response means apispec could not pin the status to a concrete
	// code — the real 4xx/5xx isn't documented. Info-level (the route still
	// works), but it feeds the resolution taxonomy and the "partial" bucket.
	if _, ok := op.Responses["default"]; ok {
		issues = append(issues, Issue{
			Severity: "info", Kind: "default-status", Method: method, Path: path,
			Detail: "an error status could not be determined (e.g. computed dynamically); the concrete 4xx/5xx isn't documented",
		})
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
