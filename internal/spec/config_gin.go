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

// ginRequestContext is the RequestContext preset for the Gin framework:
// handlers receive a *gin.Context whose Request field carries the body.
var ginRequestContext = RequestContextConfig{
	TypeRegexes: []string{
		`^\*?github\.com/gin-gonic/gin\.Context$`,
		`^\*?net/http\.Request$`,
	},
	BodyAccessors: []string{
		`^Request\.Body$`,
		`^Body$`,
	},
}

// DefaultGinConfig returns a default configuration for the Gin framework.
func DefaultGinConfig() *APISpecConfig {
	// gin's request context, the receiver of its param/bind/response helpers.
	const ginContextRecv = "^github\\.com/gin-gonic/gin\\.\\*Context$"

	responsePatterns := netHTTPResponsePatterns()
	responsePatterns = append(responsePatterns,
		// Scoped to gin's Context: this reads the status from arg 0, which is a
		// gin convention — unscoped it would misread a status-less call like
		// fiber's c.JSON(obj), which is why SecondaryView dropped it and a
		// secondary gin lost its responses (issue #211).
		ResponsePattern{
			CallRegex:      `^(?i)(JSON|String|XML|YAML|ProtoBuf|Data|File|Redirect)$`,
			StatusArgIndex: 0,
			TypeArgIndex:   1,
			TypeFromArg:    true,
			StatusFromArg:  true,
			RecvTypeRegex:  ginContextRecv,
		},
		jsonEncodePattern(""),
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
					RecvTypeRegex:   "^github\\.com/gin-gonic/gin\\.\\*(Engine|RouterGroup)$",
				},
			},
			RequestContext: ginRequestContext,
			RequestBodyPatterns: []RequestBodyPattern{
				{
					CallRegex:     `^(?i)(BindJSON|ShouldBindJSON|BindXML|BindYAML|BindForm|ShouldBind)$`,
					TypeArgIndex:  0,
					TypeFromArg:   true,
					Deref:         true,
					RecvTypeRegex: ginContextRecv,
				},
				jsonDecodeRequestPattern(""),
				jsonUnmarshalRequestPattern(""),
			},
			ResponsePatterns: responsePatterns,
			// Receiver-scoped so they survive SecondaryView when gin is not the
			// primary framework: unscoped, every one of these was dropped and a
			// secondary gin documented its endpoints with no parameters at all
			// (issue #211). The scope is gin's own Context, which is the only
			// receiver these calls ever had.
			ParamPatterns: []ParamPattern{
				{
					CallRegex:     "^Param$",
					ParamIn:       "path",
					ParamArgIndex: 0,
					RecvTypeRegex: ginContextRecv,
				},
				{
					CallRegex:     "^Query$",
					ParamIn:       "query",
					ParamArgIndex: 0,
					RecvTypeRegex: ginContextRecv,
				},
				{
					CallRegex:     "^DefaultQuery$",
					ParamIn:       "query",
					ParamArgIndex: 0,
					RecvTypeRegex: ginContextRecv,
				},
				{
					CallRegex:     "^GetHeader$",
					ParamIn:       "header",
					ParamArgIndex: 0,
					RecvTypeRegex: ginContextRecv,
				},
			},
			SecurityPatterns: ginSecurityPatterns(),
			MountPatterns: []MountPattern{
				{
					CallRegex:      `^Group$`,
					PathFromArg:    true,
					RouterFromArg:  true,
					PathArgIndex:   0,
					RouterArgIndex: 1,
					IsMount:        true,
					RecvTypeRegex:  "^github\\.com/gin-gonic/gin\\.\\*(Engine|RouterGroup)$",
				},
			},
		},
		Defaults: stdDefaults(http.StatusOK),
		ExternalTypes: []ExternalType{
			{
				Name: "github.com/gin-gonic/gin.H",
				OpenAPIType: &Schema{
					Type: "object",
				},
			},
		},
	}
}
