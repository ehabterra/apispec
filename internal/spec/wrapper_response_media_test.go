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

import (
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// A derived responder's body role carries the media type the inner call gives
// the body, or — for forwarded raw bytes — is a body only where the call site
// serialized it (issue #645).
//
// Three responders on one context, each forwarding (status, body):
//
//	PlainText: WriteHeader(status); Write(bs)          — raw bytes
//	Fail:      http.Error(w, msg, status)              — text/plain by name
//	Marshal:   bs := json.Marshal(v); Write(bs)        — a serializer's document
//
// The raw write's media type is decided inside the method — by the Content-Type
// it declares, or the server's sniff — so lifting its argument unconditionally
// documented the bytes under the JSON default at every call site; it is marked
// SerializedBodyOnly instead, since a caller that marshals first still sends a
// document. http.Error names its media type, which the lifted body lost the
// same way. The serializer's bytes are the value's document and keep their body
// role: that is the negative.
func TestDerivedResponderBodyMediaType(t *testing.T) {
	pool := metadata.NewStringPool()
	m := &metadata.Metadata{StringPool: pool, CurrentModulePath: "example.com/app"}

	ident := func(name, typ string) metadata.CallArgument {
		a := metadata.NewCallArgument(m)
		a.SetKind(metadata.KindIdent)
		a.SetName(name)
		if typ != "" {
			a.SetType(typ)
		}
		return *a
	}
	arg := func(name string) *metadata.CallArgument { a := ident(name, ""); return &a }
	sig := func(params ...metadata.CallArgument) metadata.CallArgument {
		s := metadata.NewCallArgument(m)
		s.SetKind(metadata.KindFuncType)
		for i := range params {
			s.Args = append(s.Args, &params[i])
		}
		return *s
	}
	call := func(name, pkg, recv string) metadata.Call {
		c := metadata.Call{Meta: m, Name: pool.Get(name), Pkg: pool.Get(pkg), Position: -1, Scope: -1, SignatureStr: -1, RecvType: -1}
		if recv != "" {
			c.RecvType = pool.Get(recv)
		}
		return c
	}

	// bs := json.Marshal(v)
	marshal := metadata.NewCallArgument(m)
	marshal.SetKind(metadata.KindCall)
	marshal.Fun = arg("Marshal")
	marshal.Args = []*metadata.CallArgument{arg("v")}
	marshaled := map[string][]metadata.Assignment{
		"bs": {{Value: *marshal, CalleeFunc: "Marshal", CalleePkg: "encoding/json"}},
	}

	m.Packages = map[string]*metadata.Package{
		"example.com/app": {Files: map[string]*metadata.File{
			"ctx.go": {Types: map[string]*metadata.Type{
				"Ctx": {Name: pool.Get("Ctx"), Methods: []metadata.Method{
					{Name: pool.Get("PlainText"), Signature: sig(ident("status", "int"), ident("bs", "[]byte"))},
					{Name: pool.Get("Fail"), Signature: sig(ident("status", "int"), ident("msg", "string"))},
					{Name: pool.Get("Marshal"), Signature: sig(ident("status", "int"), ident("v", "any")), AssignmentMap: marshaled},
				}},
			}},
		}},
	}
	m.CallGraph = []metadata.CallGraphEdge{
		{Caller: call("PlainText", "example.com/app", "*Ctx"), Callee: call("WriteHeader", "net/http", "ResponseWriter"), Args: []*metadata.CallArgument{arg("status")}},
		{Caller: call("PlainText", "example.com/app", "*Ctx"), Callee: call("Write", "net/http", "ResponseWriter"), Args: []*metadata.CallArgument{arg("bs")}},
		{Caller: call("Fail", "example.com/app", "*Ctx"), Callee: call("Error", "net/http", ""), Args: []*metadata.CallArgument{arg("w"), arg("msg"), arg("status")}},
		{Caller: call("Marshal", "example.com/app", "*Ctx"), Callee: call("WriteHeader", "net/http", "ResponseWriter"), Args: []*metadata.CallArgument{arg("status")}},
		{Caller: call("Marshal", "example.com/app", "*Ctx"), Callee: call("Write", "net/http", "ResponseWriter"), Args: []*metadata.CallArgument{arg("bs")}, AssignmentMap: marshaled},
	}
	m.BuildCallGraphMaps()

	const textPlain = "text/plain; charset=utf-8"
	cfg := &APISpecConfig{Framework: FrameworkConfig{
		ResponseContext: ResponseContextConfig{
			BodyTransforms: []BodyTransform{{CallRegex: `^Marshal$`, PkgRegex: `^encoding/json$`, ArgIndex: 0}},
		},
		ResponsePatterns: []ResponsePattern{
			{CallRegex: `^WriteHeader$`, StatusFromArg: true, StatusArgIndex: 0, TypeArgIndex: -1},
			{CallRegex: `^Write$`, TypeFromArg: true, TypeArgIndex: 0, RawBody: true},
			{CallRegex: `^Error$`, StatusFromArg: true, StatusArgIndex: 2, TypeFromArg: true, TypeArgIndex: 1, DefaultContentType: textPlain},
		},
	}}

	byMethod := map[string]DetectedWrapper{}
	for _, w := range DetectRouterWrappers(m, cfg) {
		byMethod[w.Methods[0]] = w
	}

	pt, ok := byMethod["PlainText"]
	if !ok || pt.Response == nil || !pt.Complete {
		t.Fatalf("PlainText not derived: a caller may hand it serialized bytes: %+v", pt)
	}
	if !pt.Response.TypeFromArg || pt.Response.TypeArgIndex != 1 {
		t.Errorf("PlainText body role = (%v, %d), want the bs parameter", pt.Response.TypeFromArg, pt.Response.TypeArgIndex)
	}
	if !pt.Response.SerializedBodyOnly || !pt.Response.RawBody {
		t.Errorf("PlainText (serializedBodyOnly, rawBody) = (%v, %v), want both: the bytes are a body only where the caller serialized them",
			pt.Response.SerializedBodyOnly, pt.Response.RawBody)
	}

	fail, ok := byMethod["Fail"]
	if !ok || fail.Response == nil || !fail.Complete {
		t.Fatalf("Fail not derived: %+v", fail)
	}
	if !fail.Response.TypeFromArg || fail.Response.TypeArgIndex != 1 {
		t.Errorf("Fail body role = (%v, %d), want the msg parameter", fail.Response.TypeFromArg, fail.Response.TypeArgIndex)
	}
	if fail.Response.DefaultContentType != textPlain {
		t.Errorf("Fail media type = %q, want %q — http.Error's own", fail.Response.DefaultContentType, textPlain)
	}

	mar, ok := byMethod["Marshal"]
	if !ok || mar.Response == nil || !mar.Complete {
		t.Fatalf("Marshal not derived: a serializer's bytes are the value's document, not raw: %+v", mar)
	}
	if !mar.Response.TypeFromArg || mar.Response.TypeArgIndex != 1 {
		t.Errorf("Marshal body role = (%v, %d), want the v parameter", mar.Response.TypeFromArg, mar.Response.TypeArgIndex)
	}
	if mar.Response.DefaultContentType != "" {
		t.Errorf("Marshal media type = %q, want the default", mar.Response.DefaultContentType)
	}
	if mar.Response.SerializedBodyOnly || mar.Response.RawBody {
		t.Errorf("Marshal (serializedBodyOnly, rawBody) = (%v, %v), want neither: its bytes are already a document",
			mar.Response.SerializedBodyOnly, mar.Response.RawBody)
	}
}

// A body declined because it collided with the status takes the media type it
// was lifted with along: the pattern no longer documents a body to carry one.
func TestNormaliseResponseRolesClearsMediaType(t *testing.T) {
	w := &DetectedWrapper{Response: &ResponsePattern{
		StatusFromArg: true, StatusArgIndex: 0, TypeFromArg: true, TypeArgIndex: 0,
		DefaultContentType: "text/plain", RawBody: true, SerializedBodyOnly: true,
	}}
	normaliseResponseRoles(w)
	if w.Response.DefaultContentType != "" {
		t.Errorf("DefaultContentType = %q after the body was declined, want empty", w.Response.DefaultContentType)
	}
	if w.Response.RawBody || w.Response.SerializedBodyOnly {
		t.Errorf("(rawBody, serializedBodyOnly) = (%v, %v) after the body was declined, want neither",
			w.Response.RawBody, w.Response.SerializedBodyOnly)
	}
}
