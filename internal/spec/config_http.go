package spec

import "net/http"

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
					PathArgIndex:    0,
					MethodArgIndex:  -1,
					HandlerArgIndex: 1,
					RecvTypeRegex:   "^net/http(\\.\\*ServeMux)?$",
				},
				{
					CallRegex:       `^Handle$`,
					PathFromArg:     true,
					HandlerFromArg:  true,
					PathArgIndex:    0,
					MethodArgIndex:  -1,
					HandlerArgIndex: 1,
				},
			},
			RequestContext: netHTTPRequestContext,
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
					CallRegex:     "^Get$",
					ParamIn:       "header",
					ParamArgIndex: 0,
				},
				{
					CallRegex:     "^Cookie$",
					ParamIn:       "cookie",
					ParamArgIndex: 0,
				},
			},
		},
		Defaults: stdDefaults(http.StatusOK),
	}
}
