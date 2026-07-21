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

import "net/http"

// netHTTPRequestContext is the RequestContext preset for plain net/http
// handlers. Chi and Mux share it because their handlers also bind to
// *http.Request; both refer to it directly within this package.
var netHTTPRequestContext = RequestContextConfig{
	TypeRegexes:   []string{`^\*?net/http\.Request$`},
	BodyAccessors: []string{`^Body$`},
}

// netHTTPResponseContext is the ResponseContext preset for the net/http family
// (net/http, chi, mux — all encode to an http.ResponseWriter). The response
// writer is the handler's `w http.ResponseWriter` parameter; an encode is a
// response only when its destination traces back to it. The compatible list
// keeps `func writeJSON(w io.Writer, v)` helpers whose destination stays an
// io.Writer interface (could be the writer).
var netHTTPResponseContext = ResponseContextConfig{
	// Only the handler's parameter type. Independently-constructible concretes
	// (httptest.ResponseRecorder, net/http.response) are intentionally NOT
	// listed: a locally-built recorder is writer-typed but is not the handler's
	// response writer, so encoding to it must not count as the response
	// (provenance, not type — CodeRabbit review on PR #181).
	WriterTypeRegexes: []string{
		`^net/http\.ResponseWriter$`,
	},
	WriterCompatibleTypeRegexes: []string{
		`^io\.Writer$`,
		`^io\.WriteCloser$`,
		`^io\.ReadWriter$`,
	},
	// Serializers whose result, when written to the response writer, carries the
	// response body's type on their payload argument (issue #195). Serializer-
	// level, not framework-level, so every net/http-family framework shares it.
	BodyTransforms: []BodyTransform{
		{CallRegex: `^Marshal$`, PkgRegex: `^encoding/json$`, ArgIndex: 0},
		{CallRegex: `^MarshalIndent$`, PkgRegex: `^encoding/json$`, ArgIndex: 0},
	},
}

// DefaultHTTPConfig returns a default configuration for net/http.
func DefaultHTTPConfig() *APISpecConfig {
	// net/http response patterns come from netHTTPResponsePatterns(); the
	// only HTTP-specific renderer is the (?i)(JSON|String|XML|...) catch-all
	// for the helper packages that wrap ResponseWriter.
	responsePatterns := netHTTPResponsePatterns()
	responsePatterns = append(responsePatterns,
		ResponsePattern{
			CallRegex:      `^(?i)(JSON|String|XML|YAML|ProtoBuf|Data|File|Redirect)$`,
			StatusArgIndex: 0,
			TypeArgIndex:   1,
			TypeFromArg:    true,
			Deref:          true,
		},
		jsonEncodePattern(""),
	)

	return &APISpecConfig{
		Framework: FrameworkConfig{
			// A handler passed as a value (r.Handle("/x", h)) is invoked through
			// http.Handler; without this its body is unreachable (issue #204).
			HandlerInterfaceMethods: []string{"ServeHTTP"},
			RoutePatterns: []RoutePattern{
				{
					CallRegex:       `^HandleFunc$`,
					PathFromArg:     true,
					HandlerFromArg:  true,
					MethodFromPath:  true,
					PathArgIndex:    0,
					MethodArgIndex:  -1,
					HandlerArgIndex: 1,
					RecvTypeRegex:   "^net/http(\\.\\*ServeMux)?$",
				},
				{
					CallRegex:       `^Handle$`,
					PathFromArg:     true,
					HandlerFromArg:  true,
					MethodFromPath:  true,
					PathArgIndex:    0,
					MethodArgIndex:  -1,
					HandlerArgIndex: 1,
				},
			},
			SecurityPatterns: httpSecurityPatterns(),
			RequestContext:   netHTTPRequestContext,
			ResponseContext:  netHTTPResponseContext,
			MountPatterns: []MountPattern{
				{
					CallRegex:      `^Handle$`,
					PathFromArg:    true,
					RouterFromArg:  true,
					PathArgIndex:   0,
					RouterArgIndex: 1,
					IsMount:        true,
					RecvTypeRegex:  `^net/http(\.\*ServeMux)?$`,
					// Only a mounted ROUTER, never an ordinary handler (issue #138).
					RouterArgTypeRegex: `^\*?(github\.com/go-chi/chi(/v\d)?\.(Mux|Router)|github\.com/gorilla/mux\.Router|net/http\.ServeMux|github\.com/labstack/echo(/v\d)?\.Echo|github\.com/gin-gonic/gin\.(Engine|RouterGroup)|github\.com/gofiber/fiber(/v\d)?\.App)$`,
				},
			},
			RequestBodyPatterns: []RequestBodyPattern{
				jsonDecodeRequestPattern(""),
				jsonUnmarshalRequestPattern(""),
			},
			ResponsePatterns: responsePatterns,
			ParamPatterns: []ParamPattern{
				{
					CallRegex:     "^FormValue$",
					ParamIn:       "form",
					ParamArgIndex: 0,
				},
				{
					// r.Header.Get("X-Foo") — scope to the http.Header
					// receiver so package-level funcs that happen to be named
					// Get (e.g. http.Get(url), client.Get(url)) are not
					// mistaken for header reads. See body_source/sync.
					CallRegex:     "^Get$",
					ParamIn:       "header",
					ParamArgIndex: 0,
					RecvType:      "net/http.Header",
				},
				{
					// r.URL.Query().Get("q") — query parameter. Query()
					// returns net/url.Values, whose Get reads a query key.
					CallRegex:     "^Get$",
					ParamIn:       "query",
					ParamArgIndex: 0,
					RecvType:      "net/url.Values",
				},
				{
					CallRegex:     "^Cookie$",
					ParamIn:       "cookie",
					ParamArgIndex: 0,
				},
				{
					// Go 1.22 ServeMux path wildcards: id := r.PathValue("id")
					CallRegex:     "^PathValue$",
					ParamIn:       "path",
					ParamArgIndex: 0,
					RecvType:      "net/http.*Request",
				},
			},
		},
		Defaults: stdDefaults(http.StatusOK),
	}
}
