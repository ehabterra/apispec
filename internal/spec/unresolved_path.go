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
)

// UnresolvedPathRoute is a registration that was found and understood except
// for the one thing an operation cannot be written without: its path.
//
// It is reported rather than emitted (issue #428). A path built at runtime —
// from a table of routes, or by a house router that carries the pattern on an
// object — used to be documented under the placeholder standing in for the
// expression, `/{Method} {Path}` or `/{pattern}`. That is worse than a missing
// route in three concrete ways: it fails a spec-lint gate on an otherwise clean
// project, it generates a client method for an endpoint that does not exist,
// and — since the real routes are missing at the same time — the path count
// stays plausible enough for the loss to go unnoticed.
type UnresolvedPathRoute struct {
	// Path is the placeholder path that would have been emitted, e.g.
	// "/{Method} {Path}". Kept because it names the expression that could not be
	// read: the placeholder is the called function or the field read.
	Path string

	// Reason says which way the path failed, since the two need different
	// answers from the reader: a path nothing is known about is a wiring style
	// to register statically, while an invalid template means the verb went
	// unread too.
	Reason string

	// Method is the HTTP method the registration resolved to, which is often
	// the ExtractRoute default rather than a stated verb.
	Method string

	// Handler is the handler identity, so the registration can be found by what
	// it serves when the position is not recoverable.
	Handler string

	// Position is where the registration is written ("file:line"), the
	// actionable half of the report.
	Position string
}

// dropUnresolvedPathRoutes splits the routes into the ones that can be
// documented and the ones whose own path never resolved.
//
// Order is preserved so the report reads in the order the walk found them, and
// the kept slice is only reallocated when something is actually dropped — the
// overwhelmingly common case is that nothing is.
func dropUnresolvedPathRoutes(routes []*RouteInfo) ([]*RouteInfo, []UnresolvedPathRoute) {
	dropped := 0
	for _, route := range routes {
		if _, bad := undocumentablePath(route); bad {
			dropped++
		}
	}
	if dropped == 0 {
		return routes, nil
	}

	kept := make([]*RouteInfo, 0, len(routes)-dropped)
	reports := make([]UnresolvedPathRoute, 0, dropped)
	for _, route := range routes {
		reason, bad := undocumentablePath(route)
		if !bad {
			kept = append(kept, route)
			continue
		}
		reports = append(reports, UnresolvedPathRoute{
			Path:     route.OpenAPIPath(),
			Reason:   reason,
			Method:   route.Method,
			Handler:  route.Function,
			Position: routeRegistrationPosition(route),
		})
	}
	return kept, reports
}

// undocumentablePath reports whether the route cannot be given an operation,
// and why. Two ways, both about the path APISpec would emit:
//
//  1. Nothing about the location is known — the route's own path resolved to
//     placeholders and so did the prefix it is mounted at. `/{pattern}` from a
//     house router that carries the pattern on a returned object.
//
//  2. The path is not a valid template because it still contains whitespace,
//     which is what a registration whose VERB is unreadable collapses to:
//     `rt.Method+" "+rt.Path` leaves `{Method} {Path}`, a key no request can
//     match. Checked independently of the first, since the mount prefix may
//     well be literal (`/api/{Method} {Path}`) — a resolved prefix does not
//     rescue a path that cannot be a path.
//
// Deliberately NOT dropped: a route whose own path is unreadable under a prefix
// that IS known. `/repo/{username}/{reponame}/info/lfs/{path}` is gitea's
// catch-all `Any()` registration seen through its wrapper — the tail is
// approximate, but the endpoint is there and locatable, and its responses are
// real. Measured: judging on the route's own path alone deleted three such
// operations from gitea's spec.
func undocumentablePath(route *RouteInfo) (string, bool) {
	if route == nil {
		return "", false
	}
	final := route.OpenAPIPath()
	if route.PathUnresolved && pathIsAllPlaceholders(final, route.DynamicParams) {
		return "no part of its path could be read", true
	}
	if strings.ContainsAny(final, " \t\n") {
		return "the path is not a valid template (the registration's verb could not be read either)", true
	}
	return "", false
}

// routeRegistrationPosition renders "file:line" for the registration call, or
// "" when the route carries no node to read it from (a hand-built RouteInfo in
// a test, or a route completed from something other than a call).
func routeRegistrationPosition(route *RouteInfo) string {
	if route == nil || route.Node == nil {
		return ""
	}
	file, line, _ := calleePosition(route.Node)
	if file == "" || line <= 0 {
		return file
	}
	return file + ":" + strconv.Itoa(line)
}

// pathIsAllPlaceholders reports whether a path contributes nothing but the
// placeholders synthesized for expressions that could not be read — so the
// registration says which handler serves it and nothing about where.
//
// The test is deliberately about the route's OWN path rather than the path it
// is finally emitted at. An unresolved MOUNT prefix leaves a real endpoint
// whose location is unknown (`/{mountPoint}/clear` — the route is "/clear",
// mounted somewhere this analysis could not follow), and an unresolved prefix
// within the path itself leaves the literal tail (`/{dynamicBase}/dyn`, issue
// #274). Both are worth documenting with the placeholder flagged. A route whose
// own path is nothing but a placeholder is not: there is no endpoint there to
// document, at any prefix.
//
// Separators do not count as content: `/{pattern}/` says no more than
// `{pattern}` does.
func pathIsAllPlaceholders(path string, dynamic []string) bool {
	if path == "" || len(dynamic) == 0 {
		return false
	}
	rest := path
	for _, name := range dynamic {
		rest = strings.ReplaceAll(rest, "{"+name+"}", "")
	}
	return strings.Trim(rest, "/ \t") == ""
}
