package spec

import (
	"sort"
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
)

// MiddlewareRef is the resolved identity of a single middleware value applied
// to a router/route (the function, method, or constructor used as middleware).
// It is what SecurityMapping matches against, and what the diagnostics list
// reports for unresolved middleware.
type MiddlewareRef struct {
	FunctionName string `json:"functionName"` // e.g. "authMiddleware", "New", "Timeout"
	Pkg          string `json:"pkg"`          // e.g. "app/handler", "github.com/golang-jwt/..."
	RecvType     string `json:"recvType"`     // receiver type for method values (e.g. "Handler"); empty otherwise
	Position     string `json:"position"`     // source position, for diagnostics
}

// String renders a human-readable identity for logs / the UI diagnostics list.
func (r MiddlewareRef) String() string {
	var b strings.Builder
	if r.Pkg != "" {
		b.WriteString(r.Pkg)
		b.WriteByte('.')
	}
	if r.RecvType != "" {
		b.WriteString("(" + r.RecvType + ").")
	}
	if r.FunctionName != "" {
		b.WriteString(r.FunctionName)
	} else {
		b.WriteString("<anonymous>")
	}
	return b.String()
}

// empty reports whether no identity could be resolved.
func (r MiddlewareRef) empty() bool {
	return r.FunctionName == "" && r.Pkg == "" && r.RecvType == ""
}

// middlewareRefFromArg resolves a call argument used as middleware into a
// MiddlewareRef. It handles the three forms seen in real code (verified on
// testdata/complex_chi_router):
//   - ident      customMiddleware          -> name + pkg
//   - selector   h.authMiddleware          -> sel name + pkg + receiver type
//   - call       middleware.Timeout(60*…)  -> constructor name + pkg (from Fun)
//
// It returns ok=false when the argument is nil or yields no usable identity.
func middlewareRefFromArg(arg *metadata.CallArgument) (MiddlewareRef, bool) {
	if arg == nil {
		return MiddlewareRef{}, false
	}
	ref := MiddlewareRef{Position: arg.GetPosition()}

	switch arg.GetKind() {
	case metadata.KindIdent:
		ref.FunctionName = arg.GetName()
		ref.Pkg = arg.GetPkg()

	case metadata.KindSelector:
		if arg.Sel != nil {
			ref.FunctionName = arg.Sel.GetName()
			ref.Pkg = arg.Sel.GetPkg()
		}
		if ref.Pkg == "" {
			ref.Pkg = arg.GetPkg()
		}
		if arg.ReceiverType != nil {
			ref.RecvType = arg.ReceiverType.GetName()
		}

	case metadata.KindCall:
		// Constructor / wrapper call: the identity is the called function (Fun),
		// which is itself an ident (New) or a selector (pkg.New / x.Method).
		fn := arg.Fun
		if fn == nil {
			return ref, false
		}
		switch fn.GetKind() {
		case metadata.KindSelector:
			if fn.Sel != nil {
				ref.FunctionName = fn.Sel.GetName()
				ref.Pkg = fn.Sel.GetPkg()
			}
			if ref.Pkg == "" {
				ref.Pkg = fn.GetPkg()
			}
			if fn.ReceiverType != nil {
				ref.RecvType = fn.ReceiverType.GetName()
			}
		default:
			ref.FunctionName = fn.GetName()
			ref.Pkg = fn.GetPkg()
		}

	default:
		return ref, false
	}

	if ref.empty() {
		return ref, false
	}
	return ref, true
}

// anyMappingMatches reports whether any mapping resolves the ref to a scheme.
func anyMappingMatches(ref MiddlewareRef, mappings []SecurityMapping) bool {
	for _, m := range mappings {
		if m.matches(ref) {
			return true
		}
	}
	return false
}

// middlewareBaseID renders the call-graph BaseID for a middleware identity, used
// to look up the function's body edges in Metadata.Callers ("pkg.name" or
// "pkg.RecvType.name"). Returns "" when the identity is incomplete.
func middlewareBaseID(ref MiddlewareRef) string {
	if ref.Pkg == "" || ref.FunctionName == "" {
		return ""
	}
	if ref.RecvType != "" {
		return ref.Pkg + "." + strings.TrimPrefix(ref.RecvType, "*") + "." + ref.FunctionName
	}
	return ref.Pkg + "." + ref.FunctionName
}

// matches reports whether the mapping's identity matchers all match the ref.
// Empty matcher fields are ignored; a mapping with no matchers never reaches
// here (validateSecurityConfig rejects it).
func (m SecurityMapping) matches(ref MiddlewareRef) bool {
	if m.FunctionNameRegex != "" {
		if re, err := cachedRegex(m.FunctionNameRegex); err != nil || !re.MatchString(ref.FunctionName) {
			return false
		}
	}
	if m.PkgRegex != "" {
		if re, err := cachedRegex(m.PkgRegex); err != nil || !re.MatchString(ref.Pkg) {
			return false
		}
	}
	if m.RecvTypeRegex != "" {
		if re, err := cachedRegex(m.RecvTypeRegex); err != nil || !re.MatchString(ref.RecvType) {
			return false
		}
	}
	return true
}

// resolveSecurity maps a set of detected middleware to OpenAPI security
// requirements via the configured mappings.
//
// Semantics:
//   - schemes from all matched mappings are ANDed: their requirement objects'
//     keys merge into one requirement object (e.g. bearer middleware + apiKey
//     middleware on the same scope => {bearerAuth, apiKeyAuth}).
//   - each SchemesAnyOf group contributes one alternative requirement object
//     (OR), appended as a separate entry in the returned list.
//   - public is true if any matched mapping is marked Public; the caller decides
//     precedence (a public scope yields `security: []`).
//   - middleware with no matching mapping is returned in unresolved for
//     diagnostics; nothing is emitted for it.
//
// The returned reqs is nil when nothing resolved (so callers can distinguish
// "no security" from "explicitly public").
func resolveSecurity(refs []MiddlewareRef, mappings []SecurityMapping) (reqs []SecurityRequirement, public bool, unresolved []MiddlewareRef) {
	combined := SecurityRequirement{}
	var alternatives []SecurityRequirement

	for _, ref := range refs {
		matched := false
		for _, m := range mappings {
			if !m.matches(ref) {
				continue
			}
			matched = true
			if m.Public {
				public = true
			}
			for _, reqObj := range m.Schemes {
				for k, v := range reqObj {
					// Non-nil empty slice so it renders as `[]` (OpenAPI requires
					// an array of scopes), not null.
					combined[k] = append([]string{}, v...)
				}
			}
			for _, grp := range m.SchemesAnyOf {
				alt := SecurityRequirement{}
				for _, reqObj := range grp {
					for k, v := range reqObj {
						alt[k] = append([]string{}, v...)
					}
				}
				if len(alt) > 0 {
					alternatives = append(alternatives, alt)
				}
			}
		}
		if !matched {
			unresolved = append(unresolved, ref)
		}
	}

	if len(combined) > 0 {
		reqs = append(reqs, combined)
	}
	reqs = append(reqs, alternatives...)
	reqs = dedupSecurityRequirements(reqs)
	return reqs, public, unresolved
}

// dedupSecurityRequirements removes duplicate requirement objects, preserving
// first-seen order. Two objects are equal when they have the same scheme names
// each with the same (ordered) scope list.
func dedupSecurityRequirements(reqs []SecurityRequirement) []SecurityRequirement {
	if len(reqs) <= 1 {
		return reqs
	}
	seen := make(map[string]struct{}, len(reqs))
	out := reqs[:0]
	for _, r := range reqs {
		key := securityRequirementKey(r)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, r)
	}
	return out
}

// securityRequirementKey builds a stable, order-independent key for a
// requirement object so duplicates can be detected.
func securityRequirementKey(r SecurityRequirement) string {
	names := make([]string, 0, len(r))
	for k := range r {
		names = append(names, k)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		b.WriteString(n)
		b.WriteByte('=')
		b.WriteString(strings.Join(r[n], ","))
		b.WriteByte(';')
	}
	return b.String()
}
