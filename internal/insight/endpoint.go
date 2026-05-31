package insight

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
	"github.com/ehabterra/apispec/internal/spec"
)

// filePosRe finds a "<path>.go:<line>" inside a larger string — handles
// both a bare position index and a func-literal base-ID that embeds its
// position (e.g. "pkg.FuncLit:/abs/file.go:55:28").
var filePosRe = regexp.MustCompile(`(/[^\s:]+\.go):(\d+)`)

// extractFilePos returns "file.go:line" from any string containing one,
// or "" if none is present.
func extractFilePos(s string) string {
	m := filePosRe.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	return m[1] + ":" + m[2]
}

// ReqInfo / RespInfo / ParamInfo are the spec-derived shape of one route.
type ReqInfo struct {
	ContentType string `json:"contentType"`
	Schema      string `json:"schema"`
	Required    bool   `json:"required"`
}
type RespInfo struct {
	Status      string `json:"status"`
	ContentType string `json:"contentType"`
	Schema      string `json:"schema"`
}
type ParamInfo struct {
	Name     string `json:"name"`
	In       string `json:"in"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

// EndpointReport is the per-route insight payload.
type EndpointReport struct {
	Method       string      `json:"method"`
	Path         string      `json:"path"`
	Found        bool        `json:"found"`        // operation exists in the spec
	Handler      string      `json:"handler"`      // operationId
	HandlerPos   string      `json:"handlerPos"`   // file:line of the handler (best effort)
	HandlerFound bool        `json:"handlerFound"` // located in the call graph
	Tags         []string    `json:"tags"`
	Summary      string      `json:"summary"`
	Request      *ReqInfo    `json:"request"`
	Responses    []RespInfo  `json:"responses"`
	Params       []ParamInfo `json:"params"`
	Issues       []Issue     `json:"issues"`
	Metrics      Metrics     `json:"metrics"`
	Trace        TraceGraph  `json:"trace"`
	TraceSource  string      `json:"traceSource"` // "tracker" | "callgraph" — which structure backed the trace
}

func operationFor(pi spec.PathItem, method string) *spec.Operation {
	switch strings.ToUpper(method) {
	case "GET":
		return pi.Get
	case "POST":
		return pi.Post
	case "PUT":
		return pi.Put
	case "DELETE":
		return pi.Delete
	case "PATCH":
		return pi.Patch
	case "OPTIONS":
		return pi.Options
	case "HEAD":
		return pi.Head
	}
	return nil
}

func componentsMap(s *spec.OpenAPISpec) map[string]*spec.Schema {
	m := map[string]*spec.Schema{}
	if s != nil && s.Components != nil {
		for k, v := range s.Components.Schemas {
			m[k] = v
		}
	}
	return m
}

// BuildEndpoint projects one route (method+path) into an EndpointReport:
// spec-derived request/response/params + issues, and — when the handler
// can be located in the call graph — Tier-1 metrics and a route-scoped
// trace. Degrades gracefully (Found/HandlerFound flags) when inputs are
// missing.
// traceSource selects which call structure backs the route trace + metrics.
const (
	TraceSourceTracker   = "tracker"   // interface-resolved tracker tree (default)
	TraceSourceCallGraph = "callgraph" // raw call graph
)

func BuildEndpoint(s *spec.OpenAPISpec, meta *metadata.Metadata, method, path string) *EndpointReport {
	return BuildEndpointWithSource(s, meta, nil, method, path, TraceSourceTracker)
}

// BuildEndpointWithSource is BuildEndpoint with an explicit trace source. The
// tracker tree (default) resolves interfaces/generics; "callgraph" uses the
// raw edges. cfg enables the tracker tree (route→handler resolution); when nil
// or the route isn't found, it falls back to the call graph.
func BuildEndpointWithSource(s *spec.OpenAPISpec, meta *metadata.Metadata, cfg *spec.APISpecConfig, method, path, traceSource string) *EndpointReport {
	rep := &EndpointReport{
		Method:    strings.ToUpper(method),
		Path:      path,
		Tags:      []string{},
		Responses: []RespInfo{},
		Params:    []ParamInfo{},
		Issues:    []Issue{},
		Trace:     TraceGraph{Nodes: []TraceNode{}, Edges: []TraceEdge{}},
	}
	if s == nil {
		return rep
	}
	pi, ok := s.Paths[path]
	if !ok {
		return rep
	}
	op := operationFor(pi, rep.Method)
	if op == nil {
		return rep
	}
	rep.Found = true
	rep.Handler = op.OperationID
	rep.Summary = op.Summary
	if op.Tags != nil {
		rep.Tags = op.Tags
	}

	// request
	if op.RequestBody != nil {
		ct, mt := firstContent(op.RequestBody.Content)
		if ct != "" {
			rep.Request = &ReqInfo{ContentType: ct, Schema: schemaSummary(mt.Schema), Required: op.RequestBody.Required}
		}
	}
	// responses (sorted by status)
	statuses := make([]string, 0, len(op.Responses))
	for st := range op.Responses {
		statuses = append(statuses, st)
	}
	sort.Strings(statuses)
	for _, st := range statuses {
		resp := op.Responses[st]
		ct, mt := firstContent(resp.Content)
		rep.Responses = append(rep.Responses, RespInfo{Status: st, ContentType: ct, Schema: schemaSummary(mt.Schema)})
	}
	// params (path-level + operation-level)
	for _, p := range append(append([]spec.Parameter{}, pi.Parameters...), op.Parameters...) {
		rep.Params = append(rep.Params, ParamInfo{
			Name: p.Name, In: p.In, Required: p.Required, Type: schemaSummary(p.Schema),
		})
	}

	// issues for this operation
	comps := componentsMap(s)
	rep.Issues = collectOperationIssues(rep.Method, path, op, comps, map[string]int{}, map[string]int{}, map[string]int{})
	if rep.Issues == nil {
		rep.Issues = []Issue{}
	}

	// call-graph metrics + trace
	if meta != nil {
		if meta.Callers == nil {
			meta.BuildCallGraphMaps()
		}
		key, pos := resolveHandlerKey(meta, strings.TrimSpace(op.OperationID))
		if edges, ok := meta.Callers[key]; ok && len(edges) > 0 {
			rep.HandlerFound = true
			rep.HandlerPos = pos
			if rep.HandlerPos == "" {
				rep.HandlerPos = extractFilePos(meta.StringPool.GetString(edges[0].Caller.Position))
			}
			// Tracker tree (default) walks the handler's scoped subtree and
			// descends through interface calls into concrete implementations.
			// Falls back to the raw call graph when the handler isn't in the
			// tree. "callgraph" forces the raw graph only.
			rep.TraceSource = TraceSourceCallGraph
			if traceSource != TraceSourceCallGraph {
				if mtr, tg, ok := analyzeFromTrackerTree(meta, cfg, rep.Method, path, key); ok {
					rep.Metrics, rep.Trace, rep.TraceSource = mtr, tg, TraceSourceTracker
				}
			}
			if rep.TraceSource == TraceSourceCallGraph {
				rep.Metrics, rep.Trace = analyzeFromHandler(meta, key)
			}
		}
	}
	return rep
}

// resolveHandlerKey maps an operationId to the call-graph caller key
// whose outgoing edges form the route's subtree, plus the handler's
// source position.
//
// Direct match wins (inline func-literal handlers, named functions). For
// the common Go pattern where the route handler is the closure returned
// by a method — `func (h Handler) X() http.HandlerFunc { return func(...) }`
// — the method itself isn't a caller; the returned FuncLit is, keyed by
// its file:line. The method and its closure live in the same file, so we
// resolve the method's declaration file and pick the FuncLit caller in
// that file at/after the method line.
func resolveHandlerKey(meta *metadata.Metadata, operationID string) (string, string) {
	if edges, ok := meta.Callers[operationID]; ok && len(edges) > 0 {
		return operationID, extractFilePos(meta.StringPool.GetString(edges[0].Caller.Position))
	}
	// The operationId's receiver/access path can differ from the concrete
	// caller key (e.g. "Handlers.AuthorHandler.GetAuthors" vs the real
	// "authorHandler.GetAuthors"). Scan the package for a method/func with
	// the operation's symbol name and collect concrete candidate keys.
	keys, file, line := candidateHandlerKeys(meta, operationID)
	pos := ""
	if file != "" {
		pos = file + ":" + itoa(line)
	}
	// Prefer a candidate that is itself a caller (direct method/func handler).
	for _, k := range keys {
		if edges, ok := meta.Callers[k]; ok && len(edges) > 0 {
			return k, pos
		}
	}
	// Otherwise the handler is a closure the method returns — find the
	// FuncLit declared in the same file (the func-returns-handler pattern).
	if file != "" {
		if fk := findFuncLitInFile(meta, file, line); fk != "" {
			return fk, pos
		}
	}
	// Interface handler: the operationId names an interface method whose
	// concrete implementation (and the closure it may return) lives in a
	// different package — resolve via the metadata's ImplementedBy index.
	if ik, ipos := resolveInterfaceImplHandler(meta, operationID); ik != "" {
		return ik, ipos
	}
	return operationID, pos
}

// resolveInterfaceImplHandler maps an interface-method operationId
// ("pkg.Iface.Method") to a concrete trace root: the implementer's method when
// it is itself a caller, or the func literal that method returns (the
// handler-factory pattern). The implementer may live in a different package
// than the interface, so this consults the ImplementedBy index rather than
// scanning a single package. Returns ("", "") when nothing resolves.
func resolveInterfaceImplHandler(meta *metadata.Metadata, operationID string) (string, string) {
	pkg, recv, name := splitOpID3(operationID)
	if pkg == "" || recv == "" || name == "" {
		return "", ""
	}
	sp := meta.StringPool
	p, ok := meta.Packages[pkg]
	if !ok {
		return "", ""
	}
	for _, f := range p.Files {
		t, ok := f.Types[recv]
		if !ok || sp.GetString(t.Kind) != "interface" {
			continue
		}
		for _, idx := range t.ImplementedBy {
			implName := sp.GetString(idx) // "import/path.Type"
			dot := strings.LastIndex(implName, ".")
			if dot <= 0 || dot == len(implName)-1 {
				continue
			}
			implPkg, implType := implName[:dot], implName[dot+1:]
			concreteKey := implPkg + "." + implType + "." + name
			mfile, mline := implMethodPos(meta, implPkg, implType, name)
			mpos := ""
			if mfile != "" {
				mpos = mfile + ":" + itoa(mline)
			}
			// Direct: the implementer method is itself a caller.
			if edges, ok := meta.Callers[concreteKey]; ok && len(edges) > 0 {
				return concreteKey, mpos
			}
			// Factory: the implementer method returns a closure — the FuncLit
			// declared in its file is the real handler body.
			if mfile != "" {
				if fk := findFuncLitInFile(meta, mfile, mline); fk != "" {
					return fk, mpos
				}
			}
		}
	}
	return "", ""
}

// splitOpID3 parses "pkg/path.Recv.Method" into package path, receiver type and
// method name. For a plain function ("pkg/path.Func") recv is "". The receiver
// and package-name segments are plain identifiers, so the only ambiguous "."
// is inside the import path, which is handled by splitting after the last "/".
func splitOpID3(opID string) (pkg, recv, method string) {
	slash := strings.LastIndex(opID, "/")
	head, rest := "", opID
	if slash >= 0 {
		head, rest = opID[:slash+1], opID[slash+1:]
	}
	parts := strings.Split(rest, ".")
	if len(parts) < 2 {
		return "", "", ""
	}
	pkg = head + parts[0]
	method = parts[len(parts)-1]
	if len(parts) >= 3 {
		recv = parts[len(parts)-2]
	}
	return pkg, recv, method
}

// implMethodPos returns the file:line of method `name` on type implType in
// package implPkg (empty when not found).
func implMethodPos(meta *metadata.Metadata, implPkg, implType, name string) (string, int) {
	sp := meta.StringPool
	p, ok := meta.Packages[implPkg]
	if !ok {
		return "", 0
	}
	for _, f := range p.Files {
		t, ok := f.Types[implType]
		if !ok {
			continue
		}
		for i := range t.Methods {
			if sp.GetString(t.Methods[i].Name) == name {
				return parsePos(sp.GetString(t.Methods[i].Position))
			}
		}
	}
	return "", 0
}

// splitOpID parses "pkg/path.Recv.Method" (or "pkg/path.Func") into the
// package path and the trailing symbol (method/func) name. The receiver
// type is dropped — the operationId sometimes capitalises it
// (e.g. "Handler") while the analysed type is unexported ("handler"), so
// we match on package + symbol name rather than the exact base-ID.
func splitOpID(opID string) (pkg, name string) {
	slash := strings.LastIndex(opID, "/")
	head, rest := "", opID
	if slash >= 0 {
		head, rest = opID[:slash+1], opID[slash+1:]
	}
	dot := strings.Index(rest, ".")
	if dot < 0 {
		return "", ""
	}
	pkg = head + rest[:dot]
	tail := rest[dot+1:] // "Recv.Method" or "Func"
	name = tail
	if i := strings.LastIndex(tail, "."); i >= 0 {
		name = tail[i+1:]
	}
	return pkg, name
}

// candidateHandlerKeys scans package `pkg` for a method or function named
// the operation's symbol (receiver-insensitive) and returns concrete
// call-graph base IDs to try (pkg.Type.method / pkg.func) plus the
// declaration file:line of the first match.
func candidateHandlerKeys(meta *metadata.Metadata, operationID string) (keys []string, file string, line int) {
	pkg, name := splitOpID(operationID)
	if pkg == "" || name == "" {
		return nil, "", 0
	}
	sp := meta.StringPool
	scanType := func(pkgPath string, t *metadata.Type) {
		tn := sp.GetString(t.Name)
		for i := range t.Methods {
			if sp.GetString(t.Methods[i].Name) == name {
				keys = append(keys, pkgPath+"."+tn+"."+name)
				if file == "" {
					file, line = parsePos(sp.GetString(t.Methods[i].Position))
				}
			}
		}
	}
	for pkgPath, p := range meta.Packages {
		if pkgPath != pkg {
			continue
		}
		for _, t := range p.Types {
			scanType(pkgPath, t)
		}
		for _, f := range p.Files {
			for _, t := range f.Types {
				scanType(pkgPath, t)
			}
			for _, fn := range f.Functions {
				if sp.GetString(fn.Name) == name {
					keys = append(keys, pkgPath+"."+name)
					if file == "" {
						file, line = parsePos(sp.GetString(fn.Position))
					}
				}
			}
		}
	}
	return keys, file, line
}

// findFuncLitInFile returns the FuncLit caller key declared in file at or
// after afterLine (closest), or "".
func findFuncLitInFile(meta *metadata.Metadata, file string, afterLine int) string {
	best := ""
	bestLine := 1 << 30
	for k := range meta.Callers {
		if !strings.Contains(k, "FuncLit:") {
			continue
		}
		f, l := parsePos(extractFilePos(k))
		if f != file || l < afterLine {
			continue
		}
		if l < bestLine {
			bestLine = l
			best = k
		}
	}
	return best
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

// firstContent returns a content entry, preferring application/json.
func firstContent(content map[string]spec.MediaType) (string, spec.MediaType) {
	if mt, ok := content["application/json"]; ok {
		return "application/json", mt
	}
	for ct, mt := range content {
		return ct, mt
	}
	return "", spec.MediaType{}
}

// schemaSummary renders a compact, human-readable description of a schema
// (ref name, wrapper allOf, array, or primitive).
func schemaSummary(sc *spec.Schema) string {
	if sc == nil {
		return ""
	}
	if strings.HasPrefix(sc.Ref, refPrefix) {
		return shortName(strings.TrimPrefix(sc.Ref, refPrefix))
	}
	if len(sc.AllOf) > 0 {
		parts := make([]string, 0, len(sc.AllOf))
		for _, p := range sc.AllOf {
			if p == nil {
				continue
			}
			if strings.HasPrefix(p.Ref, refPrefix) {
				parts = append(parts, shortName(strings.TrimPrefix(p.Ref, refPrefix)))
			} else if len(p.Properties) > 0 {
				parts = append(parts, "{"+strings.Join(propKeys(p), ", ")+"}")
			}
		}
		return "allOf[" + strings.Join(parts, " + ") + "]"
	}
	if sc.Type == "array" {
		return "[]" + schemaSummary(sc.Items)
	}
	if sc.Type != "" {
		if sc.Format != "" {
			return sc.Type + " (" + sc.Format + ")"
		}
		return sc.Type
	}
	if len(sc.Properties) > 0 {
		return "object{" + strings.Join(propKeys(sc), ", ") + "}"
	}
	return "object"
}

// shortName trims a sanitised component key (slashes/dots → underscores)
// to its trailing symbol for display, e.g.
// "github_com_..._dtos_ListTransactionResponse" → "ListTransactionResponse".
func shortName(s string) string {
	if i := strings.LastIndex(s, "_"); i >= 0 {
		return s[i+1:]
	}
	if i := strings.LastIndex(s, "."); i >= 0 {
		return s[i+1:]
	}
	return s
}

func propKeys(sc *spec.Schema) []string {
	keys := make([]string, 0, len(sc.Properties))
	for k := range sc.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > 5 {
		keys = append(keys[:5], "…")
	}
	return keys
}
