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

// signalsAuth reports whether a middleware — or anything it calls — shows a
// sign of doing authentication.
//
// This is what separates the warning that matters from the noise around it. A
// middleware that maps to no security scheme is reported so the user can map
// it, because the consequence is severe: its routes are documented as PUBLIC.
// But EVERY middleware in a Use/group/per-route slot reached that report, and a
// normal service is mostly logging, recovery, request-id, CORS, compression,
// rate limiting and timeouts. Recognised libraries are already skipped by
// securitySkipBundles; a project's own middleware matched nothing, so all of it
// was announced as "auth middleware not mapped" — 6 warnings where 2 were real
// on the service that reported this, and 16 on defaults. A warning that is
// mostly false trains people past the true one (issue #520).
//
// Deciding from the BODY rather than from the name is what keeps this from
// being a denylist of "logger"/"cors" (golden rule #9): a house middleware
// called `guard` is judged by whether it reads a credential, not by what it is
// called. Nothing is silently dropped either — a middleware this cannot
// classify is still reported, at verbose level, which is why the rule can be
// this strict without trading a false positive for a false negative.
//
// Two independent signals, either sufficient: the middleware READS a credential,
// or it REFUSES with a status that means "not authenticated/authorised". They
// catch different things, and each alone leaves real auth middleware silent:
//
//	reads Authorization, writes 401     both
//	reads a cookie, REDIRECTS to /login credential only — session auth writes no 401
//	reads a header, returns an error    credential only — the status is decided elsewhere
//	reads X-Acme-Request-Signature, 403 status only — no name table can know that one
//
// Memoized per function, and walks the same two edge sets as
// middlewareMatchesThrough: the calls in the body, and the calls inside func
// literals it defines — which is where a wrapper's real work lives, since a
// middleware IS a function returning a handler.
func (e *Extractor) signalsAuth(ref MiddlewareRef, meta *metadata.Metadata) bool {
	key := middlewareBaseID(ref)
	if key == "" || meta == nil {
		return false
	}
	e.ensureParentFnIndex(meta)
	return e.authSignalThrough(key, meta, make(map[string]bool, 8), 0)
}

// credentialWalkDepth bounds the walk. An auth middleware shows its signal
// close to the surface — the header read is in the middleware, or in the one
// helper it delegates to — and the bound keeps a deep call graph from being
// searched exhaustively for every unmapped middleware on every route.
const credentialWalkDepth = 4

func (e *Extractor) authSignalThrough(key string, meta *metadata.Metadata, seen map[string]bool, depth int) bool {
	if depth > credentialWalkDepth || seen[key] {
		return false
	}
	seen[key] = true
	if e.mwReadsCred[key] {
		return true
	}

	found := false
	scan := func(edges []*metadata.CallGraphEdge) {
		for _, edge := range edges {
			if found || edge == nil {
				continue
			}
			if e.callSignalsAuth(edge) {
				found = true
				continue
			}
			callee := e.calleeMiddlewareRef(edge)
			if callee.empty() {
				continue
			}
			if calleeKey := middlewareBaseID(callee); calleeKey != "" {
				found = e.authSignalThrough(calleeKey, meta, seen, depth+1)
			}
		}
	}
	scan(meta.Callers[key])
	scan(e.parentFnIndex[key])

	// Only a POSITIVE is memoized. A negative depends on the state this walk
	// was in when it reached the function — the remaining depth, and which
	// functions were already on the path — so a helper first visited at the
	// depth bound, or through a cycle, answers "no signal" for reasons that do
	// not hold for the next middleware to reach it by a shorter route. Caching
	// that would make an auth middleware silent because an unrelated one was
	// classified first. A positive is state-independent: the signal is there.
	if found {
		if e.mwReadsCred == nil {
			e.mwReadsCred = make(map[string]bool)
		}
		e.mwReadsCred[key] = true
	}
	return found
}

// callSignalsAuth reports whether one call shows an auth signal: it reads a
// credential, or it refuses the request with an auth status.
func (e *Extractor) callSignalsAuth(edge *metadata.CallGraphEdge) bool {
	cred := e.cfg.Framework.CredentialReads
	if cred.empty() {
		return false
	}
	name := e.contextProvider.GetString(edge.Callee.Name)
	pkg := e.contextProvider.GetString(edge.Callee.Pkg)
	recv := e.contextProvider.GetString(edge.Callee.RecvType)

	// 1. A call that IS the credential read whatever it is passed:
	// `r.BasicAuth()`, `c.Cookie(...)`, a JWT extractor.
	for _, acc := range cred.Accessors {
		if matchesCall(acc, name, pkg, recv) {
			return true
		}
	}

	// 2. A call that READS something by name, whose name is a credential:
	// `r.Header.Get("Authorization")`, `c.GetHeader("X-Api-Key")`. The call has
	// to be a read — a literal on its own is not evidence, or
	// `log.Debug("Authorization")` and a proxy middleware SETTING an outbound
	// Authorization header would both read as authentication.
	if e.callNamesCredential(edge, cred, name, pkg, recv) {
		return true
	}

	// 3. A call that REFUSES with an auth status. Gated on the framework's own
	// response patterns, which are already the list of calls that write a
	// status and where they carry it — so `http.Error(w, msg, 403)` counts and
	// `metrics.Observe(403)` does not.
	return e.callRefusesWithAuthStatus(edge, cred)
}

// callNamesCredential reports whether this call reads something by name and the
// name is a credential.
func (e *Extractor) callNamesCredential(edge *metadata.CallGraphEdge, cred CredentialReadConfig, name, pkg, recv string) bool {
	if len(cred.NameRegexes) == 0 || len(cred.NamedReads) == 0 {
		return false
	}
	isRead := false
	for _, rd := range cred.NamedReads {
		if matchesCall(rd, name, pkg, recv) {
			isRead = true
			break
		}
	}
	if !isRead {
		return false
	}
	for _, arg := range edge.Args {
		if arg == nil || arg.GetKind() != metadata.KindLiteral {
			continue
		}
		lit := strings.Trim(arg.GetValue(), "\"`")
		if lit == "" {
			continue
		}
		for _, re := range cred.NameRegexes {
			if compiled, err := cachedRegex(re); err == nil && compiled.MatchString(lit) {
				return true
			}
		}
	}
	return false
}

// callRefusesWithAuthStatus reports whether this call writes an auth refusal
// status.
//
// Which calls write a status, and where they carry it, is what
// FrameworkConfig.ResponsePatterns already declares — `http.Error`'s status is
// argument 2, `WriteHeader`'s is 0, gin's `AbortWithStatus`'s is 0. Reusing
// them keeps this framework-agnostic (golden rule #5) and means a 403 passed to
// anything else — a metric, a log, a comparison — is not a refusal.
func (e *Extractor) callRefusesWithAuthStatus(edge *metadata.CallGraphEdge, cred CredentialReadConfig) bool {
	if len(cred.RefusalStatuses) == 0 {
		return false
	}
	name := e.contextProvider.GetString(edge.Callee.Name)
	pkg := e.contextProvider.GetString(edge.Callee.Pkg)
	// A response pattern scopes a PACKAGE-level writer by putting the package
	// in RecvTypeRegex — net/http's `Error`, `NotFound` and `Redirect` are all
	// written `RecvTypeRegex: ^net/http$` — and a package-level call records no
	// receiver. Reading the field literally therefore matched none of them, and
	// `http.Error(w, msg, 403)` stopped counting as a refusal.
	scope := e.contextProvider.GetString(edge.Callee.RecvType)
	if scope == "" {
		scope = pkg
	}

	for _, p := range e.cfg.Framework.ResponsePatterns {
		if !p.StatusFromArg || p.StatusArgIndex < 0 || p.StatusArgIndex >= len(edge.Args) {
			// A pattern with a FIXED status (net/http's NotFound) says the
			// status without an argument; none of those is 401 or 403.
			continue
		}
		if !matchesCall(CredentialAccessor{
			CallRegex:     p.CallRegex,
			RecvTypeRegex: p.RecvTypeRegex,
		}, name, pkg, scope) {
			continue
		}
		if code, ok := statusArgValue(edge.Args[p.StatusArgIndex]); ok && cred.refuses(code) {
			return true
		}
	}
	return false
}

// statusArgValue reads an HTTP status out of an argument, in the two spellings
// a handler writes one: the bare number, and net/http's constant.
//
// The constant is resolved by NAME rather than by following it to its
// declaration — HTTPStatusByName is the same table the schema mapper uses, and
// it is what makes `http.StatusUnauthorized` and a plain 401 the same fact
// here. A selector is matched on its trailing identifier, so an aliased import
// (`nethttp.StatusForbidden`) resolves too.
func statusArgValue(arg *metadata.CallArgument) (int, bool) {
	switch arg.GetKind() {
	case metadata.KindLiteral:
		code, err := strconv.Atoi(strings.Trim(arg.GetValue(), "\"`"))
		if err != nil {
			return 0, false
		}
		return code, true
	case metadata.KindIdent:
		code, ok := HTTPStatusByName[arg.GetName()]
		return code, ok
	case metadata.KindSelector:
		if arg.Sel == nil {
			return 0, false
		}
		code, ok := HTTPStatusByName[arg.Sel.GetName()]
		return code, ok
	}
	return 0, false
}

// matchesCall reports whether a callee matches a CredentialAccessor. An empty
// field matches anything, as everywhere else in the pattern configuration.
func matchesCall(acc CredentialAccessor, name, pkg, recv string) bool {
	if acc.CallRegex != "" {
		re, err := cachedRegex(acc.CallRegex)
		if err != nil || !re.MatchString(name) {
			return false
		}
	}
	if acc.PkgRegex != "" {
		re, err := cachedRegex(acc.PkgRegex)
		if err != nil || !re.MatchString(pkg) {
			return false
		}
	}
	if acc.RecvTypeRegex != "" {
		re, err := cachedRegex(acc.RecvTypeRegex)
		if err != nil || !re.MatchString(recv) {
			return false
		}
	}
	// A pattern that constrains nothing would match every call.
	return acc.CallRegex != "" || acc.PkgRegex != "" || acc.RecvTypeRegex != ""
}
