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
}

// DefaultEchoConfig returns a default configuration for the Echo framework.
func DefaultEchoConfig() *APISpecConfig {
	responsePatterns := netHTTPResponsePatterns()
	responsePatterns = append(responsePatterns,
		ResponsePattern{
			CallRegex:      `^(?i)(JSON|String|XML|YAML|ProtoBuf|Data|File|Redirect)$`,
			StatusArgIndex: 0,
			TypeArgIndex:   1,
			TypeFromArg:    true,
			StatusFromArg:  true,
			Deref:          true,
			RecvTypeRegex:  "github\\.com/labstack/echo/v\\d\\.Context",
		},
		ResponsePattern{
			CallRegex:      `^(?i)(NoContent)$`,
			StatusArgIndex: 0,
			StatusFromArg:  true,
			TypeArgIndex:   -1,
			RecvTypeRegex:  "github\\.com/labstack/echo/v\\d\\.Context",
		},
		jsonMarshalPattern(),
		jsonEncodePattern(".*json(iter)?\\.\\*?Encoder"),
	)

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
			ParamPatterns: []ParamPattern{
				{
					CallRegex:     "^Param$",
					ParamIn:       "path",
					ParamArgIndex: 0,
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
				},
				{
					CallRegex:     "^Cookie$",
					ParamIn:       "cookie",
					ParamArgIndex: 0,
				},
			},
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
