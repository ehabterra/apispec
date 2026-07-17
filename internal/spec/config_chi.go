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

// DefaultChiConfig returns a default configuration for the Chi router.
func DefaultChiConfig() *APISpecConfig {
	// Chi composes net/http response patterns with chi-render's JSON/Status,
	// then the generic Marshal/Encode pair. Order preserved from the
	// pre-refactor config so the matcher priority resolution is unchanged.
	responsePatterns := netHTTPResponsePatterns()
	responsePatterns = append(responsePatterns,
		ResponsePattern{
			CallRegex:     `^JSON$`,
			TypeArgIndex:  2,
			TypeFromArg:   true,
			StatusFromArg: false,
			Deref:         true,
			RecvTypeRegex: "^github\\.com/go-chi/render$",
		},
		ResponsePattern{
			CallRegex:      `^Status$`,
			StatusArgIndex: 1,
			StatusFromArg:  true,
			RecvTypeRegex:  "^github\\.com/go-chi/render$",
		},
		jsonMarshalPattern(),
		jsonEncodePattern(".*json(iter)?\\.\\*?Encoder"),
	)

	return &APISpecConfig{
		Framework: FrameworkConfig{
			RoutePatterns: []RoutePattern{
				{
					CallRegex:       `(?i)(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD)$`,
					MethodFromCall:  true,
					PathFromArg:     true,
					HandlerFromArg:  true,
					PathArgIndex:    0,
					HandlerArgIndex: 1,
					RecvTypeRegex:   "^github.com/go-chi/chi(/v\\d)?\\.\\*?(Router|Mux)$",
				},
				{
					// r.Method(http.MethodGet, "/health", handler) /
					// r.MethodFunc(http.MethodPost, "/ready", fn) — the verb is
					// the first argument.
					CallRegex:       `^Method(Func)?$`,
					PathFromArg:     true,
					HandlerFromArg:  true,
					MethodArgIndex:  0,
					PathArgIndex:    1,
					HandlerArgIndex: 2,
					RecvTypeRegex:   "^github.com/go-chi/chi(/v\\d)?\\.\\*?(Router|Mux)$",
				},
				{
					// r.Handle("/metrics", handler) / r.HandleFunc("/items", fn)
					// route EVERY verb to the handler. Emit the handler-name
					// verb when the name carries one; otherwise default (GET)
					// without marking it explicit, so a `switch r.Method`
					// handler still splits into one operation per verb.
					CallRegex:         `^Handle(Func)?$`,
					PathFromArg:       true,
					HandlerFromArg:    true,
					MethodFromHandler: true,
					MethodArgIndex:    -1,
					PathArgIndex:      0,
					HandlerArgIndex:   1,
					RecvTypeRegex:     "^github.com/go-chi/chi(/v\\d)?\\.\\*?(Router|Mux)$",
				},
			},
			RequestContext:  netHTTPRequestContext,
			ResponseContext: netHTTPResponseContext,
			RequestBodyPatterns: []RequestBodyPattern{
				{
					CallRegex:            `^DecodeJSON$`,
					TypeArgIndex:         1,
					TypeFromArg:          true,
					Deref:                true,
					RecvTypeRegex:        "^github\\.com/go-chi/render$",
					RequireRequestSource: true,
					BodySourceArgIndex:   0,
				},
				jsonDecodeRequestPattern(".*json(iter)?\\.\\*Decoder"),
				jsonUnmarshalRequestPattern("json"),
			},
			ResponsePatterns: responsePatterns,
			ParamPatterns: []ParamPattern{
				{
					CallRegex:     "^URLParam$",
					ParamIn:       "path",
					ParamArgIndex: 1,
					RecvTypeRegex: "^github\\.com/go-chi/chi(/v\\d)?$",
				},
				{
					CallRegex:     "^URLParam$",
					ParamIn:       "path",
					ParamArgIndex: 0,
					RecvTypeRegex: "^github\\.com/go-chi/chi(/v\\d)?\\.\\*?Context$",
				},
				{
					CallRegex:     "^URLParamFromCtx$",
					ParamIn:       "path",
					ParamArgIndex: 1,
					RecvTypeRegex: "^github\\.com/go-chi/chi(/v\\d)?$",
				},
				{
					CallRegex:     "^FormValue$",
					ParamIn:       "form",
					ParamArgIndex: 0,
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
			SecurityPatterns: chiSecurityPatterns(),
			MountPatterns: []MountPattern{
				{
					CallRegex:      `^Mount$`,
					PathFromArg:    true,
					RouterFromArg:  true,
					PathArgIndex:   0,
					RouterArgIndex: 1,
					IsMount:        true,
				},
				{
					CallRegex:      `^Route$`,
					PathFromArg:    true,
					RouterFromArg:  true,
					PathArgIndex:   0,
					RouterArgIndex: 1,
					IsMount:        true,
				},
			},
		},
		Defaults: stdDefaults(defaultResponseStatus),
	}
}
