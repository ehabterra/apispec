package spec

import "net/http"

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
