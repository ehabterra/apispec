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
			},
			RequestContext: netHTTPRequestContext,
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
