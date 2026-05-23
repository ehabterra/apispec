package spec

import "net/http"

// DefaultFiberConfig returns a default configuration for the Fiber framework.
func DefaultFiberConfig() *APISpecConfig {
	responsePatterns := netHTTPResponsePatterns()
	responsePatterns = append(responsePatterns,
		ResponsePattern{
			CallRegex:      `^JSON$`,
			StatusArgIndex: -1, // Fiber's c.JSON does not take status, only data
			TypeArgIndex:   0,
			TypeFromArg:    true,
			Deref:          true,
			RecvTypeRegex:  `^github\.com/gofiber/fiber(/v\d)?\.\*Ctx$`,
		},
		ResponsePattern{
			CallRegex:      `^Status$`,
			StatusArgIndex: 0,
			StatusFromArg:  true,
			TypeArgIndex:   -1,
			RecvTypeRegex:  `^github\.com/gofiber/fiber(/v\d)?\.\*Ctx$`,
		},
		ResponsePattern{
			CallRegex:      `^SendString$`,
			StatusArgIndex: -1,
			TypeArgIndex:   0,
			TypeFromArg:    true,
			RecvTypeRegex:  `^github\.com/gofiber/fiber(/v\d)?\.\*Ctx$`,
		},
		ResponsePattern{
			CallRegex:      `^SendStatus$`,
			StatusArgIndex: 0,
			TypeArgIndex:   -1,
			RecvTypeRegex:  `^github\.com/gofiber/fiber(/v\d)?\.\*Ctx$`,
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
					RecvTypeRegex:   `^github\.com/gofiber/fiber(/v\d)?\.\*?(App|Router|Group)$`,
				},
			},
			RequestContext: fiberRequestContext,
			RequestBodyPatterns: []RequestBodyPattern{
				{
					CallRegex:     `^BodyParser$`,
					TypeArgIndex:  0,
					TypeFromArg:   true,
					Deref:         true,
					RecvTypeRegex: `^github\.com/gofiber/fiber(/v\d)?\.\*?Ctx$`,
				},
				jsonDecodeRequestPattern(".*json(iter)?\\.\\*?Decoder"),
				jsonUnmarshalRequestPattern("json"),
			},
			ResponsePatterns: responsePatterns,
			ParamPatterns: []ParamPattern{
				{
					CallRegex:     "^Params$",
					ParamIn:       "path",
					ParamArgIndex: 0,
					RecvTypeRegex: `^github\.com/gofiber/fiber(/v\d)?\.\*Ctx$`,
				},
				{
					CallRegex:     "^Query$",
					ParamIn:       "query",
					ParamArgIndex: 0,
					RecvTypeRegex: `^github\.com/gofiber/fiber(/v\d)?\.\*Ctx$`,
				},
				{
					CallRegex:     "^FormValue$",
					ParamIn:       "form",
					ParamArgIndex: 0,
					RecvTypeRegex: `^github\.com/gofiber/fiber(/v\d)?\.\*Ctx$`,
				},
				{
					CallRegex:     "^Cookies$",
					ParamIn:       "cookie",
					ParamArgIndex: 0,
					RecvTypeRegex: `^github\.com/gofiber/fiber(/v\d)?\.\*Ctx$`,
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
					CallRegex:      `^Group$`,
					PathFromArg:    true,
					RouterFromArg:  true,
					PathArgIndex:   0,
					RouterArgIndex: 1,
					IsMount:        true,
					RecvTypeRegex:  `^github\.com/gofiber/fiber(/v\d)?\.\*?(App|Router|Group)$`,
				},
				{
					CallRegex:     `^Use$`,
					PathFromArg:   false,
					RouterFromArg: false,
					IsMount:       false,
					RecvTypeRegex: `^github\.com/gofiber/fiber(/v\d)?\.\*?(App|Router|Group)$`,
				},
			},
		},
		Defaults: stdDefaults(http.StatusOK),
		ExternalTypes: []ExternalType{
			{
				Name: "github.com/gofiber/fiber.Map",
				OpenAPIType: &Schema{
					Type: "object",
				},
			},
		},
	}
}
