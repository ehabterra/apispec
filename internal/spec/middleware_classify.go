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
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
)

// readsCredential reports whether a middleware — or anything it calls — takes a
// credential out of the request.
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
// Memoized per function, and walks the same two edge sets as
// middlewareMatchesThrough: the calls in the body, and the calls inside func
// literals it defines — which is where a wrapper's real work lives, since a
// middleware IS a function returning a handler.
func (e *Extractor) readsCredential(ref MiddlewareRef, meta *metadata.Metadata) bool {
	key := middlewareBaseID(ref)
	if key == "" || meta == nil {
		return false
	}
	e.ensureParentFnIndex(meta)
	return e.credentialReadThrough(key, meta, make(map[string]bool, 8), 0)
}

// credentialWalkDepth bounds the walk. An auth middleware reads its credential
// close to the surface — the header read is in the middleware, or in the one
// helper it delegates to — and the bound keeps a deep call graph from being
// searched exhaustively for every unmapped middleware on every route.
const credentialWalkDepth = 4

func (e *Extractor) credentialReadThrough(key string, meta *metadata.Metadata, seen map[string]bool, depth int) bool {
	if depth > credentialWalkDepth || seen[key] {
		return false
	}
	seen[key] = true
	if got, ok := e.mwReadsCred[key]; ok {
		return got
	}

	found := false
	scan := func(edges []*metadata.CallGraphEdge) {
		for _, edge := range edges {
			if found || edge == nil {
				continue
			}
			if e.callReadsCredential(edge) {
				found = true
				continue
			}
			callee := e.calleeMiddlewareRef(edge)
			if callee.empty() {
				continue
			}
			if calleeKey := middlewareBaseID(callee); calleeKey != "" {
				found = e.credentialReadThrough(calleeKey, meta, seen, depth+1)
			}
		}
	}
	scan(meta.Callers[key])
	scan(e.parentFnIndex[key])

	if e.mwReadsCred == nil {
		e.mwReadsCred = make(map[string]bool)
	}
	e.mwReadsCred[key] = found
	return found
}

// callReadsCredential reports whether one call takes a credential out of the
// request, by either of the two shapes a credential read has.
func (e *Extractor) callReadsCredential(edge *metadata.CallGraphEdge) bool {
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

	// 2. A call NAMING a credential: `r.Header.Get("Authorization")`,
	// `c.GetHeader("X-Api-Key")`. The name is the evidence, so it is matched on
	// the literal the call is given rather than on the call itself — a header
	// read is not an auth signal, and the header it reads is.
	if len(cred.NameRegexes) == 0 {
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
