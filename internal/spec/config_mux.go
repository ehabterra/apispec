package spec

import "net/http"

// DefaultMethodExtractionConfig returns the default verb-from-handler-name
// method extraction rules used by frameworks that don't carry the HTTP
// method on the registration call itself (Mux's HandleFunc/Handle).
func DefaultMethodExtractionConfig() *MethodExtractionConfig {
	return &MethodExtractionConfig{
		MethodMappings: []MethodMapping{
			{Patterns: []string{"get", "list", "show", "find", "fetch", "retrieve"}, Method: "GET", Priority: 10},
			{Patterns: []string{"post", "create", "add", "new", "insert"}, Method: "POST", Priority: 10},
			{Patterns: []string{"put", "update", "edit", "modify", "replace"}, Method: "PUT", Priority: 10},
			{Patterns: []string{"delete", "remove", "destroy"}, Method: "DELETE", Priority: 10},
			{Patterns: []string{"patch", "partial"}, Method: "PATCH", Priority: 10},
			{Patterns: []string{"options"}, Method: "OPTIONS", Priority: 10},
			{Patterns: []string{"head"}, Method: "HEAD", Priority: 10},
		},
		UsePrefix:        true,
		UseContains:      true,
		CaseSensitive:    false,
		DefaultMethod:    "GET",
		InferFromContext: true,
	}
}

// DefaultMuxConfig returns a default configuration for Gorilla Mux.
func DefaultMuxConfig() *APISpecConfig {
	return &APISpecConfig{
		Framework: FrameworkConfig{
			RoutePatterns: []RoutePattern{
				{
					CallRegex:        `^HandleFunc$`,
					PathFromArg:      true,
					HandlerFromArg:   true,
					PathArgIndex:     0,
					HandlerArgIndex:  1,
					RecvTypeRegex:    `^github\.com/gorilla/mux\.\*?Router$`,
					MethodExtraction: DefaultMethodExtractionConfig(),
				},
				{
					CallRegex:        `^Handle$`,
					PathFromArg:      true,
					HandlerFromArg:   true,
					PathArgIndex:     0,
					HandlerArgIndex:  1,
					RecvTypeRegex:    `^github\.com/gorilla/mux\.\*?Router$`,
					MethodExtraction: DefaultMethodExtractionConfig(),
				},
				{
					CallRegex:        `^HandlerFunc$`,
					HandlerFromArg:   true,
					HandlerArgIndex:  0,
					RecvTypeRegex:    `^github\.com/gorilla/mux\.\*?Route$`,
					MethodExtraction: DefaultMethodExtractionConfig(),
				},
				{
					CallRegex:        `^Handler$`,
					HandlerFromArg:   true,
					HandlerArgIndex:  0,
					RecvTypeRegex:    `^github\.com/gorilla/mux\.\*?Route$`,
					MethodExtraction: DefaultMethodExtractionConfig(),
				},
				{
					CallRegex:        `^Path$`,
					PathFromArg:      true,
					PathArgIndex:     0,
					RecvTypeRegex:    `^github\.com/gorilla/mux\.\*?(Router|Route)$`,
					MethodExtraction: DefaultMethodExtractionConfig(),
				},
				{
					CallRegex:         `^Methods$`,
					MethodFromHandler: true,
					MethodArgIndex:    0,
					RecvTypeRegex:     `^github\.com/gorilla/mux\.\*?(Router|Route)$`,
					MethodExtraction:  DefaultMethodExtractionConfig(),
				},
			},
			RequestContext: netHTTPRequestContext,
			RequestBodyPatterns: []RequestBodyPattern{
				jsonDecodeRequestPattern(".*json(iter)?\\.\\*?Decoder"),
				jsonUnmarshalRequestPattern("json"),
			},
			ResponsePatterns: append(netHTTPResponsePatterns(),
				jsonMarshalPattern(),
				jsonEncodePattern(".*json(iter)?\\.\\*?Encoder"),
			),
			ParamPatterns: []ParamPattern{
				// gorilla/mux exposes path variables as a map: `mux.Vars(r)["id"]`.
				// The parameter name is a map key, not a call argument, so names
				// are recovered from the string-literal index keys used on the
				// Vars(...) result (intersected with the route's `{placeholder}`
				// segments).
				{
					CallRegex:      `^Vars$`,
					ParamIn:        "path",
					NameFromMapKey: true,
					RecvTypeRegex:  `^github\.com/gorilla/mux$`,
				},
			},
			SecurityPatterns: muxSecurityPatterns(),
			MountPatterns: []MountPattern{
				{
					CallRegex:     `^PathPrefix$`,
					PathFromArg:   true,
					PathArgIndex:  0,
					IsMount:       true,
					RecvTypeRegex: `^github\.com/gorilla/mux\.\*?Router$`,
				},
				{
					CallRegex:     `^Subrouter$`,
					IsMount:       true,
					RecvTypeRegex: `^github\.com/gorilla/mux\.\*?Route$`,
				},
			},
		},
		Defaults: stdDefaults(http.StatusOK),
	}
}
