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

// netHTTPResponseContext is the ResponseContext preset for plain net/http
// handlers: the response writer is the http.ResponseWriter parameter. Chi and
// Mux share it (their handlers also take an http.ResponseWriter). The
// compatible list keeps the ubiquitous `func writeJSON(w io.Writer, v any)`
// helper shape — an io.Writer parameter could dynamically be the response
// writer, so responses written through it must not be dropped.
var netHTTPResponseContext = ResponseContextConfig{
	WriterTypeRegexes: []string{
		`^net/http\.ResponseWriter$`,
		`^\*?net/http\.response$`,
		`^\*?net/http/httptest\.ResponseRecorder$`,
	},
	WriterCompatibleTypeRegexes: []string{
		`^io\.Writer$`,
		`^io\.WriteCloser$`,
		`^io\.ReadWriter$`,
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
		jsonMarshalPattern(),
		jsonEncodePattern(""),
	)

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
