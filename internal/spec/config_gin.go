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
	BodyReaders: stdlibBodyReaders(),
}

// DefaultGinConfig returns a default configuration for the Gin framework.
func DefaultGinConfig() *APISpecConfig {
	// gin's request context, the receiver of its param/bind/response helpers.
	const ginContextRecv = "^github\\.com/gin-gonic/gin\\.\\*Context$"

	responsePatterns := netHTTPResponsePatterns()
	// Scoped to gin's Context: this reads the status from arg 0, which is a
	// gin convention — unscoped it would misread a status-less call like
	// fiber's c.JSON(obj), which is why SecondaryView dropped it and a
	// secondary gin lost its responses (issue #211).
	responsePatterns = append(responsePatterns, rendererResponsePatterns(ResponsePattern{
		StatusArgIndex: 0,
		TypeArgIndex:   1,
		TypeFromArg:    true,
		StatusFromArg:  true,
		RecvTypeRegex:  ginContextRecv,
	})...)
	responsePatterns = append(responsePatterns, nonJSONEncodePatterns()...)
	responsePatterns = append(responsePatterns, contentTypeResponsePattern(frameworkContentTypeWrites(`^\*?(github\.com/gin-gonic/gin\.)?Context$`, `^Header$`)))
	responsePatterns = append(responsePatterns, jsonEncodePattern(""))

	return &APISpecConfig{
		Framework: FrameworkConfig{
			RoutePatterns: []RoutePattern{
				{
					CallRegex:      `^(?i)(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD)$`,
					MethodFromCall: true,
					PathFromArg:    true,
					HandlerFromArg: true,
					PathArgIndex:   0,
					// gin: GET(relativePath string, handlers ...HandlerFunc) —
					// per-route middleware precedes the endpoint handler.
					HandlerArgIndex:   1,
					HandlerArgFromEnd: true,
					RecvTypeRegex:     "^github\\.com/gin-gonic/gin\\.\\*(Engine|RouterGroup)$",
				},
			},
			RequestContext: ginRequestContext,
			ResponseContext: ResponseContextConfig{
				// NO ImplicitStatus: this framework's renderers always carry a
				// status, and a test pins that. The content-type pattern
				// carries its own DefaultStatus instead, so a streamed body
				// still gets one.
				// gin: c.Header("Content-Type", v).
				ContentTypeWrites: frameworkContentTypeWrites(`^\*?(github\.com/gin-gonic/gin\.)?Context$`, `^Header$`),
				// gin's writer is `c.Writer`, of gin's own ResponseWriter
				// interface, which embeds net/http's. Both spellings, since a
				// method call records the bare type name with the path in Pkg.
				WriterTypeRegexes:           frameworkWriterTypes(`^\*?(github\.com/gin-gonic/gin\.)?ResponseWriter$`),
				WriterCompatibleTypeRegexes: writerCompatibleTypes(),
			},
			CredentialReads: frameworkCredentialReads(`^github\.com/gin-gonic/gin\.\*?Context$`),
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
			ParamPatterns: append([]ParamPattern{
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
					CallRegex:       "^DefaultQuery$",
					ParamIn:         "query",
					ParamArgIndex:   0,
					DefaultArgIndex: 1,
					RecvTypeRegex:   ginContextRecv,
				},
				{
					// The comma-ok twin of Query: the same parameter, with the
					// handler additionally checking whether it was sent.
					CallRegex:     "^GetQuery$",
					ParamIn:       "query",
					ParamArgIndex: 0,
					RecvTypeRegex: ginContextRecv,
				},
				{
					// Repeatable query keys: ?tag=a&tag=b.
					//
					// QueryMap is NOT here. It also takes a name, but returns
					// map[string]string — the deep-object form (?ids[a]=1&ids[b]=2)
					// — and an array schema would misdescribe it. That shape needs
					// its own encoding and stays out (issue #365).
					CallRegex:     "^QueryArray$",
					ParamIn:       "query",
					ParamArgIndex: 0,
					Multi:         true,
					RecvTypeRegex: ginContextRecv,
				},
				{
					CallRegex:     "^GetHeader$",
					ParamIn:       "header",
					ParamArgIndex: 0,
					RecvTypeRegex: ginContextRecv,
				},
				{
					CallRegex:     "^Cookie$",
					ParamIn:       "cookie",
					ParamArgIndex: 0,
					RecvTypeRegex: ginContextRecv,
				},
				{
					// gin's alternate path accessor. Its receiver is the Params
					// SLICE rather than the Context, which is why every
					// Context-scoped pattern above misses it.
					CallRegex:     "^ByName$",
					ParamIn:       "path",
					ParamArgIndex: 0,
					RecvTypeRegex: "^github\\.com/gin-gonic/gin\\.Params$",
				},
				{
					// gin's form-value read. The other configs all had their
					// FormValue equivalent (#171); gin's was missing, so a gin
					// form body documented no fields at all — including the text
					// parts of a multipart upload (issue #207).
					CallRegex:     "^(PostForm|GetPostForm)$",
					ParamIn:       paramInForm,
					ParamArgIndex: 0,
					RecvTypeRegex: ginContextRecv,
				},
				{
					CallRegex:       "^DefaultPostForm$",
					ParamIn:         paramInForm,
					ParamArgIndex:   0,
					DefaultArgIndex: 1,
					RecvTypeRegex:   ginContextRecv,
				},
				{
					CallRegex:     "^PostFormArray$",
					ParamIn:       paramInForm,
					ParamArgIndex: 0,
					Multi:         true,
					RecvTypeRegex: ginContextRecv,
				},
			}, ctxMultipartParamPatterns(ginContextRecv)...),
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
