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

// echoRequestContext is the RequestContext preset for the Echo framework:
// handlers receive an echo.Context whose Request() method yields the body.
var echoRequestContext = RequestContextConfig{
	TypeRegexes: []string{
		`^github\.com/labstack/echo(/v\d+)?\.Context$`,
		`^\*?net/http\.Request$`,
	},
	BodyAccessors: []string{
		`^Request\(\)\.Body$`,
		`^Body$`,
	},
	BodyReaders: stdlibBodyReaders(),
}

// DefaultEchoConfig returns a default configuration for the Echo framework.
func DefaultEchoConfig() *APISpecConfig {
	responsePatterns := netHTTPResponsePatterns()
	// echo's raw-bytes renderers state their own media type (issue #544).
	responsePatterns = append(responsePatterns, rawBodyRendererPatterns("github\\.com/labstack/echo/v\\d\\.Context",
		rawBodyCall{call: `^Blob$`, mediaTypeArg: 1, contentArg: 2},
		rawBodyCall{call: `^Stream$`, mediaTypeArg: 1, reader: true},
	)...)
	responsePatterns = append(responsePatterns, rendererResponsePatterns(ResponsePattern{
		StatusArgIndex: 0,
		TypeArgIndex:   1,
		TypeFromArg:    true,
		StatusFromArg:  true,
		Deref:          true,
		RecvTypeRegex:  "github\\.com/labstack/echo/v\\d\\.Context",
	})...)
	responsePatterns = append(responsePatterns,
		ResponsePattern{
			CallRegex:      `^(?i)(NoContent)$`,
			StatusArgIndex: 0,
			StatusFromArg:  true,
			TypeArgIndex:   -1,
			RecvTypeRegex:  "github\\.com/labstack/echo/v\\d\\.Context",
		},
	)
	responsePatterns = append(responsePatterns, nonJSONEncodePatterns()...)
	responsePatterns = append(responsePatterns, contentTypeResponsePattern(stdlibContentTypeWrites()))
	responsePatterns = append(responsePatterns, jsonEncodePattern(".*json(iter)?\\.\\*?Encoder"))

	return &APISpecConfig{
		Framework: FrameworkConfig{
			RoutePatterns: []RoutePattern{
				{
					CallRegex:       `^(?i)(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD)$`,
					MethodFromCall:  true,
					PathFromArg:     true,
					HandlerFromArg:  true,
					PathArgIndex:    0,
					HandlerArgIndex: 1,
					RecvTypeRegex:   "^github\\.com/labstack/echo(/v\\d)?\\.\\*(Echo|Group)$",
				},
			},
			RequestContext: echoRequestContext,
			ResponseContext: ResponseContextConfig{
				// NO ImplicitStatus: this framework's renderers always carry a
				// status, and a test pins that. The content-type pattern
				// carries its own DefaultStatus instead, so a streamed body
				// still gets one.
				// echo declares its media type through net/http: c.Response().Header().Set(k, v) — the stdlib entry covers it, so there is no shorthand to add.
				ContentTypeWrites: stdlibContentTypeWrites(),
				// echo's writer is `c.Response()`, an *echo.Response, which
				// implements http.ResponseWriter and is what a stream is handed.
				WriterTypeRegexes:           frameworkWriterTypes(`^\*?(github\.com/labstack/echo(/v\d+)?\.)?Response$`),
				WriterCompatibleTypeRegexes: writerCompatibleTypes(),
			},
			CredentialReads: frameworkCredentialReads(`^github\.com/labstack/echo(/v\d+)?\.Context$`),
			RequestBodyPatterns: []RequestBodyPattern{
				{
					CallRegex:     `^(?i)(Bind)$`,
					TypeArgIndex:  0,
					TypeFromArg:   true,
					Deref:         true,
					RecvTypeRegex: "github\\.com/labstack/echo/v\\d\\.Context",
				},
				jsonDecodeRequestPattern(".*json(iter)?\\.\\*Decoder"),
				jsonUnmarshalRequestPattern("json"),
			},
			ResponsePatterns: responsePatterns,
			ParamPatterns: append([]ParamPattern{
				{
					CallRegex:     "^Param$",
					ParamIn:       "path",
					ParamArgIndex: 0,
					RecvTypeRegex: "github\\.com/labstack/echo/v\\d\\.Context",
				},
				{
					CallRegex:     "^QueryParam$",
					ParamIn:       "query",
					ParamArgIndex: 0,
					RecvTypeRegex: "github\\.com/labstack/echo/v\\d\\.Context",
				},
				{
					CallRegex:     "^FormValue$",
					ParamIn:       "form",
					ParamArgIndex: 0,
					RecvTypeRegex: "github\\.com/labstack/echo/v\\d\\.Context",
				},
				{
					CallRegex:     "^Cookie$",
					ParamIn:       "cookie",
					ParamArgIndex: 0,
					RecvTypeRegex: "github\\.com/labstack/echo/v\\d\\.Context",
				},
			}, ctxMultipartParamPatterns("github\\.com/labstack/echo/v\\d\\.Context")...),
			SecurityPatterns: echoSecurityPatterns(),
			MountPatterns: []MountPattern{
				{
					CallRegex:      `^Group$`,
					PathFromArg:    true,
					RouterFromArg:  true,
					PathArgIndex:   0,
					RouterArgIndex: 1,
					IsMount:        true,
					RecvTypeRegex:  "^github\\.com/labstack/echo(/v\\d)?\\.\\*(Echo|Group)$",
				},
			},
		},
		Defaults: stdDefaults(http.StatusOK),
	}
}
