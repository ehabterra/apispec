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

package spec

import (
	"strconv"
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
)

// splitMethodDispatchRoutes expands each route whose handler dispatches on
// r.Method (a `switch r.Method` or `if r.Method == …` chain — recorded as
// metadata.Function.MethodDispatch for a declared function, metadata.LitDispatch
// for a closure, or Method.Dispatch for a method) into one route per HTTP method,
// attributing each verb branch's request and responses to its own operation.
// Routes whose handler does not dispatch pass through unchanged.
//
// Attribution is by source position, over the call sites the walk passed
// through to reach a body (ResponseInfo.EntrySites) and not only the body's own
// statement: the statement is usually written in a callee, and it is the call in
// the arm that says which verb reached it. A body with a site inside the handler
// belongs to the arms containing such a site; one whose sites are all outside
// the handler (a shared helper with no recoverable chain) is attached to every
// method; one written inside the handler but in no arm (the `default:` case's
// 405) is dropped. Two branches that return the *same* status with different
// bodies are a known limitation — the earlier status-slot pairing keeps one, so
// they don't split.
//
// A route registered with a concrete verb is not split — the router sends it
// that verb only — but it IS scoped to the matching arm: the other arms are dead
// for this route, and their bodies documented a `router.GET` operation with the
// POST arm's 201.
func splitMethodDispatchRoutes(routes []*RouteInfo) []*RouteInfo {
	out := make([]*RouteInfo, 0, len(routes))
	for _, route := range routes {
		branches, scope := methodDispatchFor(route)
		if len(branches) == 0 {
			out = append(out, route)
			continue
		}
		if route.MethodExplicit {
			out = append(out, scopeRouteToDispatchArm(route, branches, scope))
			continue
		}
		out = append(out, splitRouteByMethodBranches(route, branches, scope)...)
	}
	return out
}

// scopeRouteToDispatchArm keeps only the bodies the route's own verb produces,
// for a registration that named its verb.
//
// The route is not split and not renamed: it is one operation either way. What
// changes is that `mux.HandleFunc("GET /x", h)` with an `r.Method` switch in h
// stops documenting the POST arm's request body and 201 — arms the router never
// sends this route. When the verb has no arm at all (a handler shared across
// registrations, or a switch that does not name it) nothing is scoped away:
// the arms say nothing about this verb, so removing bodies on their say-so
// would be a guess (golden rule #7).
func scopeRouteToDispatchArm(route *RouteInfo, branches []metadata.MethodBranch, scope handlerScope) *RouteInfo {
	ranges, _ := dispatchRanges(branches)
	rs, ok := ranges[route.Method]
	if !ok {
		return route
	}
	return routeScopedToArm(route, route.Method, "", rs, scope)
}

// handlerScope is where a route's handler is written: the file, and the range
// within it the handler body occupies.
//
// The range is what tells a call site written in the handler from one written
// elsewhere in the same file — which is the whole question as soon as a function
// registers two closures, since both share the file and every arm of both is
// recorded against the enclosing declaration.
type handlerScope struct {
	file  string
	start codePos
	end   codePos
}

// contains reports whether a call site sits inside the handler's body.
func (h handlerScope) contains(p codePos) bool {
	if h.file == "" || !p.valid() || p.file != h.file {
		return false
	}
	if h.start.valid() && !h.start.beforeOrAt(p) {
		return false
	}
	return h.end.line <= 0 || p.line < h.end.line ||
		(p.line == h.end.line && (h.end.col <= 0 || p.col <= h.end.col))
}

// methodDispatchFor returns the handler's r.Method dispatch branches and where
// the handler is written, or nil when the route's handler doesn't dispatch on
// the method.
func methodDispatchFor(route *RouteInfo) ([]metadata.MethodBranch, handlerScope) {
	meta := route.Metadata
	if meta == nil || route.Function == "" {
		return nil, handlerScope{}
	}
	bare := handlerDeclName(route)
	// A closure handler is identified by where it is written, and its dispatch is
	// recorded under that identity: it has no Function record to carry it, and
	// the enclosing declaration's MethodDispatch mixes in every other literal's
	// arms (issue #382).
	if strings.HasPrefix(bare, metadata.FuncLitPrefix) {
		lit, ok := meta.LitDispatch[route.Function]
		if !ok {
			return nil, handlerScope{}
		}
		return dispatchScopeOf(meta, lit)
	}
	// A method handler's dispatch is recorded on its own declaration: methods are
	// not in file.Functions, so findFunctionByName cannot see them, and Method
	// carries no EndLine for the arms to be scoped against — hence the same
	// DispatchScope a literal gets (issue #427).
	if m := handlerMethodDecl(route, bare); m != nil {
		if m.Dispatch == nil {
			return nil, handlerScope{}
		}
		return dispatchScopeOf(meta, *m.Dispatch)
	}
	fn := findFunctionByName(meta, route.Package, bare)
	if fn == nil || len(fn.MethodDispatch) == 0 {
		return nil, handlerScope{}
	}
	pos := meta.StringPool.GetString(fn.Position)
	file, line, col := parsePosition(pos)
	return fn.MethodDispatch, handlerScope{
		file:  file,
		start: codePos{file: file, line: line, col: col},
		// EndCol 0: the declaration's last line is recorded, its column is not,
		// so the whole closing line counts as inside.
		end: codePos{file: file, line: fn.EndLine},
	}
}

// dispatchScopeOf reads a recorded dispatch into the arms and the scope the
// split works with. Empty arms answer as "no dispatch", so a body that was
// recorded but names no verb never splits a route.
func dispatchScopeOf(meta *metadata.Metadata, ds metadata.DispatchScope) ([]metadata.MethodBranch, handlerScope) {
	if len(ds.Branches) == 0 {
		return nil, handlerScope{}
	}
	file := meta.StringPool.GetString(ds.File)
	return ds.Branches, handlerScope{
		file:  file,
		start: codePos{file: file, line: ds.Body.StartLine, col: ds.Body.StartCol},
		end:   codePos{file: file, line: ds.Body.EndLine, col: ds.Body.EndCol},
	}
}

// splitRouteByMethodBranches builds one RouteInfo per HTTP method named across
// the dispatch branches, with request/response scoped to that method.
func splitRouteByMethodBranches(route *RouteInfo, branches []metadata.MethodBranch, scope handlerScope) []*RouteInfo {
	ranges, order := dispatchRanges(branches)
	if len(order) == 0 {
		return []*RouteInfo{route}
	}

	result := make([]*RouteInfo, 0, len(order))
	for _, m := range order {
		// The method is the operationId suffix too: neither verb of a split is
		// the primary one, and two operations differing only in method would
		// otherwise collide.
		result = append(result, routeScopedToArm(route, m, m, ranges[m], scope))
	}
	return result
}

// lineRange is one dispatch arm's source range, lines only (see Block: a case
// clause is compared against, where the column distinction does not arise).
type lineRange struct{ start, end int }

// dispatchRanges groups the arms by the HTTP method they name, keeping the
// methods in source order so a split is deterministic (golden rule #1). One
// method may have several arms, and one arm several methods
// (`case http.MethodGet, http.MethodHead:`).
func dispatchRanges(branches []metadata.MethodBranch) (map[string][]lineRange, []string) {
	ranges := map[string][]lineRange{}
	var order []string
	for _, b := range branches {
		for _, m := range b.Methods {
			if _, seen := ranges[m]; !seen {
				order = append(order, m)
			}
			ranges[m] = append(ranges[m], lineRange{b.StartLine, b.EndLine})
		}
	}
	return ranges, order
}

// routeScopedToArm copies the route with only the request and responses that
// belong to one verb's arms.
func routeScopedToArm(route *RouteInfo, method, idSuffix string, rs []lineRange, scope handlerScope) *RouteInfo {
	inArm := func(line int) bool {
		for _, r := range rs {
			if line >= r.start && line <= r.end {
				return true
			}
		}
		return false
	}
	// belongsTo reports whether a body reached through these call sites belongs
	// to method m. A site inside one of m's arms is decisive; a site inside the
	// handler but in no arm (the `default:` case, or code before the switch)
	// belongs to no method; and a body with no site inside the handler at all —
	// a shared helper whose chain was not recovered — is attached to every
	// method rather than dropped, since it is genuinely part of this route.
	//
	// The test is over the whole set rather than the innermost site, because a
	// slot's sites can come from several fragments: one status reached from two
	// arms is a response both arms send (see the merge in pairAndFillResponses).
	belongsTo := func(sites []codePos) bool {
		insideHandler := false
		for _, s := range sites {
			if !scope.contains(s) {
				continue
			}
			insideHandler = true
			if inArm(s.line) {
				return true
			}
		}
		return !insideHandler
	}

	nr := *route // shallow copy; Method/Request/Response are per-arm below
	nr.Method = method
	nr.OperationIDSuffix = idSuffix
	nr.Response = map[string]*ResponseInfo{}
	nr.Request = nil

	for slot, resp := range route.Response {
		if resp != nil && belongsTo(sitesOf(resp.EntrySites, resp.File, resp.Line)) {
			nr.Response[slot] = resp
		}
	}
	if route.Request != nil &&
		belongsTo(sitesOf(route.Request.EntrySites, route.Request.File, route.Request.Line)) {
		nr.Request = route.Request
	}
	return &nr
}

// sitesOf returns the call sites to attribute a body by, falling back to its own
// statement position when no chain was recorded (a body matched outside the
// route walk, or one carried over from a route that predates EntrySites).
func sitesOf(sites []codePos, file string, line int) []codePos {
	if len(sites) > 0 {
		return sites
	}
	if file == "" || line <= 0 {
		return nil
	}
	return []codePos{{file: file, line: line}}
}

// parsePosition splits a "file:line:col" position string, tolerating a Windows
// drive-letter colon the same way fileOfPosition does.
func parsePosition(pos string) (string, int, int) {
	file := fileOfPosition(pos)
	if len(pos) <= len(file) {
		return file, 0, 0
	}
	rest := strings.Split(pos[len(file)+1:], ":")
	line, _ := strconv.Atoi(rest[0])
	col := 0
	if len(rest) > 1 {
		col, _ = strconv.Atoi(rest[1])
	}
	return file, line, col
}

// fileOfPosition returns the file portion of a "file:line:col" position string,
// tolerating a Windows drive-letter colon (only the trailing :line:col is
// stripped).
func fileOfPosition(pos string) string {
	lastColon := strings.LastIndexByte(pos, ':')
	if lastColon < 0 {
		return pos
	}
	rest := pos[:lastColon]
	midColon := strings.LastIndexByte(rest, ':')
	if midColon < 0 {
		return rest
	}
	return rest[:midColon]
}

// expandMultiVerbRoutes turns a registration that named several verbs into one
// route per verb — a house router's `Methods("GET,POST", path, h)` registers both
// (issue #221).
//
// The copy is shallow on purpose: every verb of one registration shares the same
// handler, so it shares the handler's request, responses and parameters. What must
// NOT be shared is the operationId, so each verb carries it as a suffix the way a
// dispatch split does — neither verb is the primary one, and two operations
// differing only in method would otherwise collide.
func expandMultiVerbRoutes(routes []*RouteInfo) []*RouteInfo {
	out := make([]*RouteInfo, 0, len(routes))
	for _, route := range routes {
		if len(route.ExtraMethods) == 0 {
			out = append(out, route)
			continue
		}
		methods := append([]string{route.Method}, route.ExtraMethods...)
		for _, method := range methods {
			nr := *route
			nr.Method = method
			nr.ExtraMethods = nil
			nr.OperationIDSuffix = method
			out = append(out, &nr)
		}
	}
	return out
}
