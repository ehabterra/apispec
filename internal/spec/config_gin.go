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
	responsePatterns := netHTTPResponsePatterns()
	responsePatterns = append(responsePatterns,
		ResponsePattern{
			CallRegex:      `^(?i)(JSON|String|XML|YAML|ProtoBuf|Data|File|Redirect)$`,
			StatusArgIndex: 0,
			TypeArgIndex:   1,
			TypeFromArg:    true,
			StatusFromArg:  true,
		},
		jsonMarshalPattern(),
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
					CallRegex:    `^(?i)(BindJSON|ShouldBindJSON|BindXML|BindYAML|BindForm|ShouldBind)$`,
					TypeArgIndex: 0,
					TypeFromArg:  true,
					Deref:        true,
				},
				jsonDecodeRequestPattern(""),
				jsonUnmarshalRequestPattern(""),
			},
			ResponsePatterns: responsePatterns,
			ParamPatterns: []ParamPattern{
				{
					CallRegex:     "^Param$",
					ParamIn:       "path",
					ParamArgIndex: 0,
				},
				{
					CallRegex:     "^Query$",
					ParamIn:       "query",
					ParamArgIndex: 0,
				},
				{
					CallRegex:     "^DefaultQuery$",
					ParamIn:       "query",
					ParamArgIndex: 0,
				},
				{
					CallRegex:     "^GetHeader$",
					ParamIn:       "header",
					ParamArgIndex: 0,
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
