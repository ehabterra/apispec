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

	"github.com/ehabterra/apispec/internal/metadata"
	"github.com/ehabterra/apispec/internal/typemodel"
)

// addReturnedSentinels documents the error sentinels a handler RETURNS
// (issue #556): `return echo.ErrForbidden`, `return fiber.ErrNotFound`. The
// framework's error handler writes the status the value carries, but the value
// is a package variable rather than a call, so no response pattern — which
// matches call edges — can see it.
//
// Only the handler's own return statements are read, through the same
// declaration resolver the doc-comment summary uses, so both agree on which
// function serves the route. A status the route already documents is left
// alone: a constructor or context write for it is the more specific statement.
func (e *Extractor) addReturnedSentinels(route *RouteInfo) {
	if e.cfg == nil || len(e.cfg.Framework.ErrorSentinels) == 0 {
		return
	}
	decl := resolveHandlerDecl(route, e.cfg.Framework.HandlerInterfaceMethods...)
	if decl == nil {
		return
	}
	for _, ret := range decl.returns {
		for i := range ret {
			resp := e.sentinelResponse(route, &ret[i])
			if resp == nil {
				continue
			}
			slot := strconv.Itoa(resp.StatusCode)
			if route.Response == nil {
				route.Response = map[string]*ResponseInfo{}
			}
			if _, documented := route.Response[slot]; documented {
				continue
			}
			route.Response[slot] = resp
		}
	}
}

// sentinelResponse returns the response a returned value produces when it is a
// configured error sentinel, or nil.
func (e *Extractor) sentinelResponse(route *RouteInfo, v *metadata.CallArgument) *ResponseInfo {
	if v.GetKind() != metadata.KindSelector || v.Sel == nil {
		return nil
	}
	name, pkg, typ := v.Sel.GetName(), v.Sel.GetPkg(), v.Sel.GetType()
	for _, s := range e.cfg.Framework.ErrorSentinels {
		code, ok := sentinelStatus(s, name, pkg, typ)
		if !ok {
			continue
		}
		contentType := s.ContentType
		if contentType == "" {
			contentType = e.cfg.Defaults.ResponseContentType
		}
		resp := &ResponseInfo{StatusCode: code, ContentType: contentType}
		bodyType := s.BodyType
		if s.BodyFromValue {
			bodyType = typ
			if ref := typemodel.Parse(bodyType); s.Deref && ref != nil && ref.Kind == typemodel.KindPointer && ref.Elem != nil {
				bodyType = ref.Elem.String()
			}
		}
		if bodyType != "" {
			resp.BodyType = preprocessingBodyType(bodyType)
			resp.Schema = mapGoTypeForRoute(route.UsedTypes, bodyType, route.Metadata, e.cfg)
		}
		return resp
	}
	return nil
}

// sentinelStatus reports the status an error sentinel carries: its package and
// type must match, and its name must capture a status name the status table
// knows. echo spells one `ErrStatusRequestEntityTooLarge`, so a capture that
// already starts with "Status" is tried as it is.
func sentinelStatus(s ErrorSentinel, name, pkg, typ string) (int, bool) {
	if !regexMatches(s.PkgRegex, pkg) || (s.TypeRegex != "" && !regexMatches(s.TypeRegex, typ)) {
		return 0, false
	}
	re, err := cachedRegex(s.NameRegex)
	if err != nil {
		return 0, false
	}
	m := re.FindStringSubmatch(name)
	if len(m) < 2 || m[1] == "" {
		return 0, false
	}
	if code, ok := HTTPStatusByName["Status"+m[1]]; ok {
		return code, true
	}
	code, ok := HTTPStatusByName[m[1]]
	return code, ok
}

// regexMatches reports whether pattern matches s; an empty or invalid pattern
// matches nothing.
func regexMatches(pattern, s string) bool {
	if pattern == "" {
		return false
	}
	re, err := cachedRegex(pattern)
	return err == nil && re.MatchString(s)
}
