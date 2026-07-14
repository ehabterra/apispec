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

// HTTPSecondaryConfig is the merge-safe subset of the net/http default config,
// meant to be layered UNDER another framework's config for mixed projects (a
// chi/gin API plus plain ServeMux ops endpoints in one binary). net/http never
// appears in go.mod and its import is near-universal, so the merge cannot be
// gated on detection — instead every contributed pattern is receiver- or
// package-scoped, making the whole config inert unless the project actually
// registers routes on net/http.
//
// Deliberately narrower than DefaultHTTPConfig:
//   - ^Handle$ is receiver-scoped here (unscoped in the primary config, where
//     it exists to catch wrapper registrars); unscoped it would double-match
//     other routers' Handle methods.
//   - the (?i)(JSON|String|XML|...) response catch-all is dropped: it reads
//     status from arg 0 and would misread status-less calls like fiber's
//     c.JSON(obj) if merged into that framework.
//   - Marshal/Encode/Decode variants use the encoder/decoder-scoped forms.
//   - FormValue/Cookie param patterns are omitted until they gain receiver
//     scoping; the scoped header/query/PathValue patterns are included.
func HTTPSecondaryConfig() *APISpecConfig {
	serveMuxRecv := "^net/http(\\.\\*ServeMux)?$"
	return &APISpecConfig{
		Framework: FrameworkConfig{
			RoutePatterns: []RoutePattern{
				{
					CallRegex:       `^HandleFunc$`,
					PathFromArg:     true,
					HandlerFromArg:  true,
					MethodFromPath:  true,
					PathArgIndex:    0,
					MethodArgIndex:  -1,
					HandlerArgIndex: 1,
					RecvTypeRegex:   serveMuxRecv,
				},
				{
					CallRegex:       `^Handle$`,
					PathFromArg:     true,
					HandlerFromArg:  true,
					MethodFromPath:  true,
					PathArgIndex:    0,
					MethodArgIndex:  -1,
					HandlerArgIndex: 1,
					RecvTypeRegex:   serveMuxRecv,
				},
			},
			SecurityPatterns: httpSecurityPatterns(),
			RequestContext:   netHTTPRequestContext,
			RequestBodyPatterns: []RequestBodyPattern{
				jsonDecodeRequestPattern(".*json(iter)?\\.\\*Decoder"),
				jsonUnmarshalRequestPattern("json"),
			},
			ResponsePatterns: netHTTPResponsePatterns(),
			ParamPatterns: []ParamPattern{
				{
					CallRegex:     "^Get$",
					ParamIn:       "header",
					ParamArgIndex: 0,
					RecvType:      "net/http.Header",
				},
				{
					CallRegex:     "^Get$",
					ParamIn:       "query",
					ParamArgIndex: 0,
					RecvType:      "net/url.Values",
				},
				{
					CallRegex:     "^PathValue$",
					ParamIn:       "path",
					ParamArgIndex: 0,
					RecvType:      "net/http.*Request",
				},
			},
		},
	}
}

// SecondaryView filters a framework config down to its merge-safe subset:
// only patterns carrying a receiver or package constraint (RecvType or
// RecvTypeRegex) survive. The rule is mechanical on purpose — a scoped
// pattern cannot claim another framework's calls, while an unscoped one
// (the generic Marshal/Encode/FormValue helpers every config repeats, or a
// catch-all like net/http's JSON responder) can misfire when transplanted.
// The primary config keeps its own copies of the shared unscoped patterns,
// so dropping them from secondaries loses nothing that was safe to merge.
// Known cost: a secondary framework's unscoped Mount-style patterns are
// dropped too, so mounts wired through a *secondary* framework's router may
// not be traced — scoping those patterns in their home config is the
// eventual fix. The receiving config is untouched; a filtered copy is
// returned.
func SecondaryView(cfg *APISpecConfig) *APISpecConfig {
	if cfg == nil {
		return nil
	}
	out := &APISpecConfig{Framework: FrameworkConfig{
		RequestContext: cfg.Framework.RequestContext,
	}}
	for _, p := range cfg.Framework.RoutePatterns {
		if p.RecvType != "" || p.RecvTypeRegex != "" {
			out.Framework.RoutePatterns = append(out.Framework.RoutePatterns, p)
		}
	}
	for _, p := range cfg.Framework.RequestBodyPatterns {
		if p.RecvType != "" || p.RecvTypeRegex != "" {
			out.Framework.RequestBodyPatterns = append(out.Framework.RequestBodyPatterns, p)
		}
	}
	for _, p := range cfg.Framework.ResponsePatterns {
		if p.RecvType != "" || p.RecvTypeRegex != "" {
			out.Framework.ResponsePatterns = append(out.Framework.ResponsePatterns, p)
		}
	}
	for _, p := range cfg.Framework.ParamPatterns {
		if p.RecvType != "" || p.RecvTypeRegex != "" {
			out.Framework.ParamPatterns = append(out.Framework.ParamPatterns, p)
		}
	}
	for _, p := range cfg.Framework.MountPatterns {
		if p.RecvType != "" || p.RecvTypeRegex != "" {
			out.Framework.MountPatterns = append(out.Framework.MountPatterns, p)
		}
	}
	for _, p := range cfg.Framework.SecurityPatterns {
		if p.RecvType != "" || p.RecvTypeRegex != "" {
			out.Framework.SecurityPatterns = append(out.Framework.SecurityPatterns, p)
		}
	}
	return out
}

// MergeFrameworkConfigs layers secondary framework configs under the primary:
// pattern lists are appended in order with first-occurrence-wins dedupe (the
// primary's variant of a pattern always beats a secondary's), and the request
// context accumulates unique type regexes and body accessors. Info, Defaults,
// overrides and mappings stay the primary's alone. The primary is mutated and
// returned.
func MergeFrameworkConfigs(primary *APISpecConfig, secondaries ...*APISpecConfig) *APISpecConfig {
	seenRoute := map[string]bool{}
	for _, p := range primary.Framework.RoutePatterns {
		seenRoute[routePatternKey(p)] = true
	}
	seenReq := map[string]bool{}
	for _, p := range primary.Framework.RequestBodyPatterns {
		seenReq[patternKey(p.CallRegex, p.RecvTypeRegex, "")] = true
	}
	seenResp := map[string]bool{}
	for _, p := range primary.Framework.ResponsePatterns {
		seenResp[patternKey(p.CallRegex, p.RecvTypeRegex, "")] = true
	}
	seenParam := map[string]bool{}
	for _, p := range primary.Framework.ParamPatterns {
		seenParam[patternKey(p.CallRegex, p.RecvTypeRegex+"\x00"+p.RecvType, p.ParamIn)] = true
	}
	seenMount := map[string]bool{}
	for _, p := range primary.Framework.MountPatterns {
		seenMount[patternKey(p.CallRegex, p.RecvTypeRegex, "")] = true
	}
	seenSec := map[string]bool{}
	for _, p := range primary.Framework.SecurityPatterns {
		seenSec[patternKey(p.CallRegex, p.RecvTypeRegex, string(p.Scope))] = true
	}

	for _, sec := range secondaries {
		if sec == nil {
			continue
		}
		for _, p := range sec.Framework.RoutePatterns {
			if k := routePatternKey(p); !seenRoute[k] {
				seenRoute[k] = true
				primary.Framework.RoutePatterns = append(primary.Framework.RoutePatterns, p)
			}
		}
		for _, p := range sec.Framework.RequestBodyPatterns {
			if k := patternKey(p.CallRegex, p.RecvTypeRegex, ""); !seenReq[k] {
				seenReq[k] = true
				primary.Framework.RequestBodyPatterns = append(primary.Framework.RequestBodyPatterns, p)
			}
		}
		for _, p := range sec.Framework.ResponsePatterns {
			if k := patternKey(p.CallRegex, p.RecvTypeRegex, ""); !seenResp[k] {
				seenResp[k] = true
				primary.Framework.ResponsePatterns = append(primary.Framework.ResponsePatterns, p)
			}
		}
		for _, p := range sec.Framework.ParamPatterns {
			if k := patternKey(p.CallRegex, p.RecvTypeRegex+"\x00"+p.RecvType, p.ParamIn); !seenParam[k] {
				seenParam[k] = true
				primary.Framework.ParamPatterns = append(primary.Framework.ParamPatterns, p)
			}
		}
		for _, p := range sec.Framework.MountPatterns {
			if k := patternKey(p.CallRegex, p.RecvTypeRegex, ""); !seenMount[k] {
				seenMount[k] = true
				primary.Framework.MountPatterns = append(primary.Framework.MountPatterns, p)
			}
		}
		for _, p := range sec.Framework.SecurityPatterns {
			if k := patternKey(p.CallRegex, p.RecvTypeRegex, string(p.Scope)); !seenSec[k] {
				seenSec[k] = true
				primary.Framework.SecurityPatterns = append(primary.Framework.SecurityPatterns, p)
			}
		}
		primary.Framework.RequestContext.TypeRegexes = appendUniqueStrings(
			primary.Framework.RequestContext.TypeRegexes, sec.Framework.RequestContext.TypeRegexes...)
		primary.Framework.RequestContext.BodyAccessors = appendUniqueStrings(
			primary.Framework.RequestContext.BodyAccessors, sec.Framework.RequestContext.BodyAccessors...)
	}
	return primary
}

// routePatternKey identifies a route pattern by its matching surface — the
// regex fields that decide WHICH calls it claims. Two patterns with the same
// surface but different extraction hints are conflicting configurations; the
// primary's wins.
func routePatternKey(p RoutePattern) string {
	return strings.Join([]string{p.CallRegex, p.FunctionNameRegex, p.RecvType, p.RecvTypeRegex}, "\x00")
}

func patternKey(parts ...string) string {
	return strings.Join(parts, "\x00")
}
