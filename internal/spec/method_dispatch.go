package spec

import (
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
)

// splitMethodDispatchRoutes expands each route whose handler dispatches on
// r.Method (a `switch r.Method` or `if r.Method == …` chain — recorded as
// metadata.Function.MethodDispatch) into one route per HTTP method, attributing
// each verb branch's request and responses to its own operation. Routes whose
// handler does not dispatch pass through unchanged.
//
// Attribution is by source position: a request/response located inside the
// handler is assigned to the branch whose line range contains it; one located
// outside the handler (a shared helper in another file, or with no recoverable
// position) is attached to every method, and one located inside the handler but
// outside every verb branch (e.g. the `default:` arm's 405) is dropped. Two
// branches that return the *same* status with different bodies are a known
// limitation — the earlier status-slot pairing keeps one, so they don't split.
func splitMethodDispatchRoutes(routes []*RouteInfo) []*RouteInfo {
	out := make([]*RouteInfo, 0, len(routes))
	for _, route := range routes {
		branches, handlerFile := methodDispatchFor(route)
		if len(branches) == 0 {
			out = append(out, route)
			continue
		}
		out = append(out, splitRouteByMethodBranches(route, branches, handlerFile)...)
	}
	return out
}

// methodDispatchFor returns the handler's r.Method dispatch branches and the
// source file the handler is defined in (for same-file position scoping), or
// nil when the route's handler doesn't dispatch on the method.
func methodDispatchFor(route *RouteInfo) ([]metadata.MethodBranch, string) {
	meta := route.Metadata
	if meta == nil || route.Function == "" {
		return nil, ""
	}
	bare := route.Function
	if route.Package != "" {
		bare = strings.TrimPrefix(route.Function, route.Package+".")
	}
	fn := findFunctionByName(meta, route.Package, bare)
	if fn == nil || len(fn.MethodDispatch) == 0 {
		return nil, ""
	}
	return fn.MethodDispatch, fileOfPosition(meta.StringPool.GetString(fn.Position))
}

// splitRouteByMethodBranches builds one RouteInfo per HTTP method named across
// the dispatch branches, with request/response scoped to that method.
func splitRouteByMethodBranches(route *RouteInfo, branches []metadata.MethodBranch, handlerFile string) []*RouteInfo {
	type lineRange struct{ start, end int }
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
	if len(order) == 0 {
		return []*RouteInfo{route}
	}

	// insideHandler reports whether a call site sits in the handler's own file
	// (so its line can be compared against the branch ranges).
	insideHandler := func(file string, line int) bool {
		return handlerFile != "" && line > 0 && file == handlerFile
	}
	inRanges := func(rs []lineRange, line int) bool {
		for _, r := range rs {
			if line >= r.start && line <= r.end {
				return true
			}
		}
		return false
	}
	// belongsTo reports whether a call site at (file,line) belongs to method m:
	// either it's outside the handler (shared → every method) or it falls in one
	// of m's branch ranges. A site inside the handler but in no branch (default
	// arm) belongs to no method.
	belongsTo := func(rs []lineRange, file string, line int) bool {
		if !insideHandler(file, line) {
			return true
		}
		return inRanges(rs, line)
	}

	result := make([]*RouteInfo, 0, len(order))
	for _, m := range order {
		rs := ranges[m]
		nr := *route // shallow copy; per-method Method/Request/Response below
		nr.Method = m
		nr.OperationIDSuffix = m // keep operationIds unique across the split
		nr.Response = map[string]*ResponseInfo{}
		nr.Request = nil

		for slot, resp := range route.Response {
			if resp != nil && belongsTo(rs, resp.File, resp.Line) {
				nr.Response[slot] = resp
			}
		}
		if route.Request != nil && belongsTo(rs, route.Request.File, route.Request.Line) {
			nr.Request = route.Request
		}
		result = append(result, &nr)
	}
	return result
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
