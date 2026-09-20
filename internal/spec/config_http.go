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

// netHTTPRequestContext is the RequestContext preset for plain net/http
// handlers. Chi and Mux share it because their handlers also bind to
// *http.Request; both refer to it directly within this package.
var netHTTPRequestContext = RequestContextConfig{
	TypeRegexes:   []string{`^\*?net/http\.Request$`},
	BodyAccessors: []string{`^Body$`},
	BodyReaders:   stdlibBodyReaders(),
}

// stdlibBodyReaders are the standard-library calls that turn a reader into
// bytes. Every framework's request context gets the same list: reaching for
// io.ReadAll before unmarshalling is a Go habit, not a router's (golden rule
// #5), and the handler writes it identically whichever router is in front.
//
// Kept as a function so no framework can mutate another's copy.
func stdlibBodyReaders() []BodyReader {
	return []BodyReader{
		{CallRegex: `^ReadAll$`, PkgRegex: `^io$`, SourceArgIndex: 0},
		// Pre-1.16 spelling, still widespread in projects that have not
		// migrated; io/ioutil.ReadAll simply forwards to io.ReadAll.
		{CallRegex: `^ReadAll$`, PkgRegex: `^io/ioutil$`, SourceArgIndex: 0},
	}
}

// netHTTPResponseContext is the ResponseContext preset for the net/http family
// (net/http, chi, mux — all encode to an http.ResponseWriter). The response
// writer is the handler's `w http.ResponseWriter` parameter; an encode is a
// response only when its destination traces back to it. The compatible list
// keeps `func writeJSON(w io.Writer, v)` helpers whose destination stays an
// io.Writer interface (could be the writer).
var netHTTPResponseContext = ResponseContextConfig{
	// Only the handler's parameter type. Independently-constructible concretes
	// (httptest.ResponseRecorder, net/http.response) are intentionally NOT
	// listed: a locally-built recorder is writer-typed but is not the handler's
	// response writer, so encoding to it must not count as the response
	// (provenance, not type — CodeRabbit review on PR #181).
	WriterTypeRegexes: []string{
		`^net/http\.ResponseWriter$`,
	},
	WriterCompatibleTypeRegexes: []string{
		`^io\.Writer$`,
		`^io\.WriteCloser$`,
		`^io\.ReadWriter$`,
	},
	// Serializers whose result, when written to the response writer, carries the
	// response body's type on their payload argument (issue #195). Serializer-
	// level, not framework-level, so every net/http-family framework shares it.
	BodyTransforms: []BodyTransform{
		{CallRegex: `^Marshal$`, PkgRegex: `^encoding/json$`, ArgIndex: 0},
		{CallRegex: `^MarshalIndent$`, PkgRegex: `^encoding/json$`, ArgIndex: 0},
	},
	// The two ways a handler moves a buffer's bytes to the writer. Both put the
	// writer at argument 0; they differ only in where the buffer is. Serializer-
	// level like BodyTransforms above, so every net/http-family framework shares
	// them (issue #471).
	BufferSinks: []BufferSink{
		// buf.WriteTo(w) — bytes.Buffer, strings.Builder and every io.WriterTo.
		{CallRegex: `^WriteTo$`, BufferFromReceiver: true, WriterArgIndex: 0},
		// io.Copy(w, buf) and io.CopyBuffer(w, buf, scratch).
		{CallRegex: `^Copy(Buffer)?$`, PkgRegex: `^io$`, WriterArgIndex: 0, BufferArgIndex: 1},
	},
	// net/http: "If WriteHeader is not called explicitly, the first call to
	// Write will trigger an implicit WriteHeader(http.StatusOK)".
	ContentTypeWrites: stdlibContentTypeWrites(),
	ImplicitStatus:    http.StatusOK,
}

// DefaultHTTPConfig returns a default configuration for net/http.
func DefaultHTTPConfig() *APISpecConfig {
	// net/http response patterns come from netHTTPResponsePatterns(); the
	// only HTTP-specific renderer is the (?i)(JSON|String|XML|...) catch-all
	// for the helper packages that wrap ResponseWriter.
	responsePatterns := netHTTPResponsePatterns()
	responsePatterns = append(responsePatterns, rendererResponsePatterns(ResponsePattern{
		StatusArgIndex: 0,
		TypeArgIndex:   1,
		TypeFromArg:    true,
		Deref:          true,
		// Anchored on the writer being SOMEWHERE in the call (issue #302).
		// Unanchored, this matched any call with one of these names in any
		// reached package — a protobuf helper, an encoding library, anything
		// named Data or File — and documented its second argument as this
		// endpoint's response. There is no fixed writer position to name here,
		// because the helpers this exists for do not share a signature.
		RequireResponseDestination: true,
		DestFromAnyArg:             true,
	})...)
	responsePatterns = append(responsePatterns, nonJSONEncodePatterns()...)
	responsePatterns = append(responsePatterns, contentTypeResponsePattern(netHTTPResponseContext.ContentTypeWrites))
	responsePatterns = append(responsePatterns, jsonEncodePattern(""))

	return &APISpecConfig{
		Framework: FrameworkConfig{
			// A handler passed as a value (r.Handle("/x", h)) is invoked through
			// http.Handler; without this its body is unreachable (issue #204).
			HandlerInterfaceMethods: []string{"ServeHTTP"},
			RoutePatterns: []RoutePattern{
				{
					CallRegex:       `^HandleFunc$`,
					PathFromArg:     true,
					HandlerFromArg:  true,
					MethodFromPath:  true,
					PathArgIndex:    0,
					MethodArgIndex:  -1,
					HandlerArgIndex: 1,
					RecvTypeRegex:   "^net/http(\\.\\*ServeMux)?$",
				},
				{
					CallRegex:       `^Handle$`,
					PathFromArg:     true,
					HandlerFromArg:  true,
					MethodFromPath:  true,
					PathArgIndex:    0,
					MethodArgIndex:  -1,
					HandlerArgIndex: 1,
					// Scoped to net/http's own receiver, exactly as ^HandleFunc$
					// above is. Without it, `Handle` matched on the NAME alone —
					// and `Handle` is what a house router calls its own
					// registration method:
					//
					//	func (c *Combo) Handle(h http.HandlerFunc) *Combo {
					//		c.r.mux.HandleFunc(c.pattern, h)
					//	}
					//
					// That took argument 0 of `Combo.Handle(listItems)` as the
					// path — the HANDLER — and documented the route at
					// `{listItems}`, while the real registration inside the
					// method was never reached. A placeholder built from a
					// handler's name is not a path any client can call
					// (issue #506).
					RecvTypeRegex: `^net/http(\.\*ServeMux)?$`,
				},
			},
			SecurityPatterns: httpSecurityPatterns(),
			RequestContext:   netHTTPRequestContext,
			CredentialReads:  stdlibCredentialReads(),
			ResponseContext:  netHTTPResponseContext,
			MountPatterns: []MountPattern{
				{
					CallRegex:      `^Handle$`,
					PathFromArg:    true,
					RouterFromArg:  true,
					PathArgIndex:   0,
					RouterArgIndex: 1,
					IsMount:        true,
					RecvTypeRegex:  `^net/http(\.\*ServeMux)?$`,
					// Only a mounted ROUTER, never an ordinary handler (issue #138).
					RouterArgTypeRegex: `^\*?(github\.com/go-chi/chi(/v\d)?\.(Mux|Router)|github\.com/gorilla/mux\.Router|net/http\.ServeMux|github\.com/labstack/echo(/v\d)?\.Echo|github\.com/gin-gonic/gin\.(Engine|RouterGroup)|github\.com/gofiber/fiber(/v\d)?\.App)$`,
				},
			},
			RequestBodyPatterns: []RequestBodyPattern{
				jsonDecodeRequestPattern(""),
				jsonUnmarshalRequestPattern(""),
			},
			ResponsePatterns: responsePatterns,
			ParamPatterns: append([]ParamPattern{
				{
					CallRegex:     "^FormValue$",
					ParamIn:       "form",
					ParamArgIndex: 0,
					// On *http.Request, not the router: scoping lets it survive
					// SecondaryView (issue #211).
					RecvType: "net/http.*Request",
				},
				{
					// r.Header.Get("X-Foo") — scope to the http.Header
					// receiver so package-level funcs that happen to be named
					// Get (e.g. http.Get(url), client.Get(url)) are not
					// mistaken for header reads. See body_source/sync.
					CallRegex:     "^Get$",
					ParamIn:       "header",
					ParamArgIndex: 0,
					RecvType:      "net/http.Header",
					// ...but only the REQUEST's headers: the same type carries the
					// response's, and `w.Header().Get(k)` reads what the server sends.
					ExcludeRecvOriginRegex: responseWriterOriginRegex,
				},
				{
					// r.Header.Values("X-Foo") — the repeatable twin of Get, and
					// the same header either way: what differs is that the client
					// may send it more than once (issue #365).
					CallRegex:              "^Values$",
					ParamIn:                "header",
					ParamArgIndex:          0,
					Multi:                  true,
					RecvType:               "net/http.Header",
					ExcludeRecvOriginRegex: responseWriterOriginRegex,
				},
				{
					// r.URL.Query().Get("q") — query parameter. Query()
					// returns net/url.Values, whose Get reads a query key.
					CallRegex:     "^Get$",
					ParamIn:       "query",
					ParamArgIndex: 0,
					RecvType:      "net/url.Values",
				},
				{
					CallRegex:     "^Cookie$",
					ParamIn:       "cookie",
					ParamArgIndex: 0,
					// On *http.Request, not the router: scoping lets it survive
					// SecondaryView (issue #211).
					RecvType: "net/http.*Request",
				},
				{
					// Go 1.22 ServeMux path wildcards: id := r.PathValue("id")
					CallRegex:     "^PathValue$",
					ParamIn:       "path",
					ParamArgIndex: 0,
					RecvType:      "net/http.*Request",
				},
			}, requestMultipartParamPatterns()...),
		},
		Defaults: stdDefaults(http.StatusOK),
	}
}

// stdlibCredentialReads is the credential surface every Go HTTP service shares,
// whatever router is in front: the names a credential travels under, and the
// stdlib calls that fetch one whatever they are passed.
//
// Used ONLY to decide whether an unmapped middleware is worth reporting as a
// missing security scheme (issue #520) — never to decide what a scheme IS, and
// never to drop a parameter. The cost of a wrong answer is a warning shown or
// withheld, and a middleware this does not recognise is still reported at
// verbose level rather than dropped, so the rule can be strict without trading
// a false positive for a silent false negative.
//
// The same table is what issue #359 needs to attach a middleware's other
// effects to the routes it guards, which is why it is config rather than code.
func stdlibCredentialReads() CredentialReadConfig {
	return CredentialReadConfig{
		Accessors: []CredentialAccessor{
			// The whole credential, with no name given.
			//
			// The receiver is matched in BOTH spellings metadata uses for it:
			// a method call on *http.Request records RecvType as the bare
			// `*Request` with the path in Pkg, while a pattern elsewhere in
			// this file sees the qualified form. Pinning one of them made
			// `r.BasicAuth()` invisible to the classifier.
			{CallRegex: `^BasicAuth$`, PkgRegex: `^net/http$`, RecvTypeRegex: `^\*?(net/http\.)?Request$`},
			// A session cookie is a credential, and the read names it.
			{CallRegex: `^Cookie$`, PkgRegex: `^net/http$`, RecvTypeRegex: `^\*?(net/http\.)?Request$`},
		},
		// Where a name has to appear to mean anything. The receiver is what
		// makes these narrow: `Get` alone would match `cache.Get("…")`.
		NamedReads: []CredentialAccessor{
			// r.Header.Get(name) — metadata renders the receiver bare or
			// qualified depending on the call, so both are accepted.
			{CallRegex: `^Get$`, RecvTypeRegex: `^\*?(net/http\.)?Header$`},
			// r.URL.Query().Get(name)
			{CallRegex: `^Get$`, RecvTypeRegex: `^\*?(net/url\.)?Values$`},
			// r.FormValue(name) / r.PostFormValue(name)
			{CallRegex: `^(FormValue|PostFormValue)$`, RecvTypeRegex: `^\*?(net/http\.)?Request$`},
		},
		NameRegexes: []string{
			`(?i)^authorization$`,
			`(?i)^proxy-authorization$`,
			// The conventional API-key spellings, matched as WHOLE names so
			// `X-Request-Id` and `X-Api-Version` are not credentials.
			`(?i)^x-(api|auth|access|session)[-_]?(key|token|secret)$`,
			`(?i)^(api|auth|access)[-_]?(key|token)$`,
			`(?i)^x-(amz-security-token|goog-api-key)$`,
		},
		// 401 is definitionally "not authenticated". 403 is authorisation,
		// which is the same family — it does mean a middleware that forbids on
		// something other than a credential (a tenancy or IP guard) is reported
		// too, and that is the accepted cost: a false positive is one line,
		// while a miss is a route documented as public.
		RefusalStatuses: []int{
			http.StatusUnauthorized,
			http.StatusForbidden,
		},
	}
}

// frameworkCredentialReads is stdlibCredentialReads plus a framework context's
// own cookie accessor — gin's and echo's `Cookie`, fiber's `Cookies` — which is
// the one credential read that carries no recognisable name of its own.
//
// Header and query accessors need no entry: they are ordinary calls, and
// callReadsCredential judges any call by the literal it is given, so
// `c.GetHeader("Authorization")` is recognised for every framework without one.
func frameworkCredentialReads(ctxRecvTypeRegex string) CredentialReadConfig {
	cred := stdlibCredentialReads()
	cred.Accessors = append(cred.Accessors, CredentialAccessor{
		CallRegex:     `^Cookies?$`,
		RecvTypeRegex: ctxRecvTypeRegex,
	})
	// The context's own by-name reads, which are the framework's spelling of
	// `r.Header.Get`. Scoped to the context type, so a same-named method on
	// anything else is not one.
	cred.NamedReads = append(cred.NamedReads, CredentialAccessor{
		CallRegex:     `^(Get|GetHeader|GetQuery|Query|QueryParam|FormValue|PostForm|Param)$`,
		RecvTypeRegex: ctxRecvTypeRegex,
	})
	return cred
}

// stdlibContentTypeWrites is how a handler declares its media type through
// net/http's header map — `w.Header().Set("Content-Type", …)`.
//
// Every framework gets this one, because every framework's response writer is
// reachable as an http.Header somewhere: echo's `c.Response().Header().Set`
// and gin's `c.Writer.Header().Set` are both this call. Frameworks that ALSO
// offer a shorthand add it beside this rather than instead of it — see
// frameworkContentTypeWrites.
//
// The receiver is matched in both spellings metadata uses, since a method call
// records the bare type name with the path in Pkg.
func stdlibContentTypeWrites() []ContentTypeWrite {
	return []ContentTypeWrite{
		{CallRegex: `^Set$`, RecvTypeRegex: `^\*?(net/http\.)?Header$`, NameArgIndex: 0, ValueArgIndex: 1},
	}
}

// frameworkContentTypeWrites adds a framework's own shorthand for the same
// declaration — gin's `c.Header(k, v)`, fiber's `c.Set(k, v)` — scoped to its
// context type so a same-named method on anything else is not one.
//
// Kept as data rather than code because this is precisely the part that
// differs per router, and a project with a house context needs to be able to
// say so in its own config rather than wait for a release.
func frameworkContentTypeWrites(ctxRecvTypeRegex, callRegex string) []ContentTypeWrite {
	out := stdlibContentTypeWrites()
	if callRegex == "" || ctxRecvTypeRegex == "" {
		return out
	}
	return append(out, ContentTypeWrite{
		CallRegex:     callRegex,
		RecvTypeRegex: ctxRecvTypeRegex,
		NameArgIndex:  0,
		ValueArgIndex: 1,
	})
}
