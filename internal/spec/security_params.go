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

import "strings"

// authorizationHeader is the header an `http` scheme (bearer, basic) and the
// OAuth2 / OpenID Connect flows carry their credential in. OpenAPI does not
// state it as a field because the scheme type implies it.
const authorizationHeader = "authorization"

// dropSchemeConsumedParams removes the header parameters that a security scheme
// on the same operation already governs (issue #412).
//
// The credential was being documented twice: once as the scheme, and once as a
// parameter the client is told to supply itself, both from the one
// `c.GetHeader("Authorization")` inside the middleware. OpenAPI treats those as
// different contracts, so a generator emitted a second, manually-supplied
// argument beside the one the scheme drives.
//
// Scoped to the operation's OWN schemes, never to a header name: a handler that
// genuinely reads `Authorization` for its own purposes, on an operation with no
// security, keeps its parameter. On a real project surveyed for the issue, the
// only Authorization parameter in the whole spec was of exactly that shape and
// correct there — which is why a name denylist would have been wrong twice over
// (golden rule #9).
func dropSchemeConsumedParams(routes []*RouteInfo, global []SecurityRequirement, schemes map[string]SecurityScheme) {
	if len(schemes) == 0 {
		return
	}
	for _, route := range routes {
		if route == nil || len(route.Params) == 0 {
			continue
		}
		consumed := schemeConsumedHeaders(effectiveRequirements(route, global), schemes)
		if len(consumed) == 0 {
			continue
		}
		kept := make([]Parameter, 0, len(route.Params))
		for _, p := range route.Params {
			if p.In == "header" && consumed[strings.ToLower(p.Name)] {
				continue
			}
			kept = append(kept, p)
		}
		if len(kept) == len(route.Params) {
			continue
		}
		// A fresh slice, never a filter in place: a route split by verb shares
		// its parameter slice with its siblings (the split copies the struct),
		// so writing through it would drop the parameter from operations this
		// decision was not made for.
		route.Params = kept
	}
}

// effectiveRequirements returns the security requirements that actually apply to
// the operation: its own when it has any, the document's when it inherits.
//
// A route with a non-nil EMPTY requirement list is explicitly public — it
// overrides the document's security — so nothing is consumed on its behalf and
// a header it reads is its own business.
func effectiveRequirements(route *RouteInfo, global []SecurityRequirement) []SecurityRequirement {
	if route.Security != nil {
		return route.Security
	}
	return global
}

// schemeConsumedHeaders returns the headers that EVERY way of authenticating
// this operation consumes.
//
// The requirement list is a set of alternatives (an OR), and each object within
// one is a set of schemes required together (an AND). So the headers are
// intersected across the alternatives: with `[{bearerAuth}, {queryKey}]` a
// client may authenticate with the query key alone and never send an
// Authorization credential, so that header is not unambiguously the scheme's
// and the parameter stays (CodeRabbit, PR #441). Dropping it would remove a
// header the handler can still require, which is the failure this whole pass is
// meant to avoid in the other direction.
//
// Names are lower-cased because HTTP header names are case-insensitive while
// the parameter and the scheme are written by different hands.
func schemeConsumedHeaders(reqs []SecurityRequirement, schemes map[string]SecurityScheme) map[string]bool {
	var consumed map[string]bool
	for i, req := range reqs {
		headers := requirementHeaders(req, schemes)
		if len(headers) == 0 {
			return nil // one alternative consumes nothing: nothing is certain
		}
		if i == 0 {
			consumed = headers
			continue
		}
		for header := range consumed {
			if !headers[header] {
				delete(consumed, header)
			}
		}
		if len(consumed) == 0 {
			return nil
		}
	}
	return consumed
}

// requirementHeaders returns the headers the schemes of ONE requirement object
// consume — they are required together, so their headers accumulate.
//
// A scheme with no definition contributes nothing: what it consumes is unknown,
// and suppressing a parameter on a guess is how a real one goes missing.
func requirementHeaders(req SecurityRequirement, schemes map[string]SecurityScheme) map[string]bool {
	var headers map[string]bool
	add := func(header string) {
		if header == "" {
			return
		}
		if headers == nil {
			headers = map[string]bool{}
		}
		headers[strings.ToLower(header)] = true
	}
	for name := range req {
		scheme, ok := schemes[name]
		if !ok {
			continue
		}
		switch strings.ToLower(scheme.Type) {
		case "http", "oauth2", "openidconnect":
			// The type implies the header; `bearerFormat` and the flows say
			// what travels in it, not where.
			add(authorizationHeader)
		case "apikey":
			// Only a header-carried key consumes a header. One in a query
			// parameter or a cookie leaves any header the handler reads as
			// genuinely its own (issue #370 made this knowable).
			if strings.EqualFold(scheme.In, "header") {
				add(scheme.Name)
			}
		}
	}
	return headers
}
