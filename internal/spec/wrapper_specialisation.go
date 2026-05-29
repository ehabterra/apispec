package spec

import (
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
)

// wrapperFieldOverride describes one field of a wrapper-style
// response struct whose concrete payload type was recovered at the
// call site through the assignment + parameter chain. StructFieldName
// is the Go field name (e.g. "Data"); the JSON name is resolved from
// the wrapper type's tags at schema-emission time.
type wrapperFieldOverride struct {
	StructFieldName string
	GoType          string
}

// collectWrapperOverrides specialises a wrapper-response body using
// nothing but the primitives that are already populated for every
// function — Function.AssignmentMap, Function.ReturnVars, and
// CallGraphEdge.ParamArgMap.
//
// The pattern we recognise:
//
//	func RespondWithSuccess(w http.ResponseWriter, msg string, data any, code int) {
//	    response := NewEnvelope(msg, data, code)
//	    json.NewEncoder(w).Encode(response)
//	}
//
//	func NewEnvelope(msg string, data any, code int) *Envelope {
//	    return &Envelope{Message: msg, Data: data, Code: code}
//	}
//
// When the response matcher matches Encode and the body arg is the
// local `response`, we want to specialise the Envelope schema with
// the caller-site type of the `data` argument. The chain is:
//
//	response ── via helper.AssignmentMap ──► NewEnvelope(msg, data, code)
//	NewEnvelope.ReturnVars[0]              ── the composite literal &Envelope{...}
//	   field "Data" is bound to NewEnvelope param "data"
//	   constructor-call arg for "data" is the helper-local ident "data"
//	   helper.ParamArgMap (one parent edge up) ──► caller's actual arg
//
// All four edges of that walk exist in the metadata already; no new
// storage is introduced.
func (r *ResponsePatternMatcherImpl) collectWrapperOverrides(arg *metadata.CallArgument, node TrackerNodeInterface) []wrapperFieldOverride {
	if arg == nil || arg.GetKind() != metadata.KindIdent || node == nil {
		return nil
	}
	edge := node.GetEdge()
	if edge == nil {
		return nil
	}
	meta := metadataFromContextProvider(r.contextProvider)
	if meta == nil {
		return nil
	}

	helper := findEnclosingFunction(meta, edge.Caller.Pkg, edge.Caller.Name)
	if helper == nil {
		return nil
	}
	assigns := helper.AssignmentMap[arg.GetName()]
	if len(assigns) == 0 {
		return nil
	}
	assign := assigns[len(assigns)-1] // latest wins, like TraceVariableOrigin
	if assign.CalleeFunc == "" || assign.CalleePkg == "" {
		return nil
	}

	ctor := findFunction(meta, assign.CalleePkg, assign.CalleeFunc)
	if ctor == nil {
		return nil
	}
	idx := assign.ReturnIndex
	if idx < 0 || idx >= len(ctor.ReturnVars) {
		return nil
	}
	bindings := fieldParamBindingsFromReturnVar(&ctor.ReturnVars[idx], ctor)
	if len(bindings) == 0 {
		return nil
	}

	ctorCallArgs := assign.Value.Args
	ctorParamNames := paramNamesOf(ctor)
	parentEdge := parentEdgeOf(node)

	out := make([]wrapperFieldOverride, 0, len(bindings))
	for fieldName, ctorParamName := range bindings {
		argAtCtor := lookupCallArgByParamName(ctorCallArgs, ctorParamNames, ctorParamName)
		if argAtCtor == nil {
			continue
		}
		concrete := r.resolveOverrideGoType(argAtCtor, parentEdge)
		if concrete == "" {
			continue
		}
		out = append(out, wrapperFieldOverride{
			StructFieldName: fieldName,
			GoType:          concrete,
		})
	}
	return out
}

// fieldParamBindingsFromReturnVar inspects a function's ReturnVars
// entry and, when the return is a composite literal of the form
// `T{Field: paramIdent, ...}` or `&T{Field: paramIdent, ...}`,
// reports which struct fields are bound directly to parameters of the
// owning function. The mapping is field-name → parameter-name.
//
// This is the same shape `extractReturnFieldBindings` would have
// captured at metadata-generation time, but recomputed on demand from
// the CallArgument tree so the metadata schema stays unchanged.
func fieldParamBindingsFromReturnVar(arg *metadata.CallArgument, ctor *metadata.Function) map[string]string {
	if arg == nil {
		return nil
	}
	cl := arg
	// Strip address-of (&T{...}) and parens.
	for cl != nil {
		switch cl.GetKind() {
		case metadata.KindUnary, metadata.KindParen:
			cl = cl.X
		default:
			goto done
		}
	}
done:
	if cl == nil || cl.GetKind() != metadata.KindCompositeLit {
		return nil
	}
	params := paramNameSetOf(ctor)
	out := map[string]string{}
	for _, elt := range cl.Args {
		if elt == nil || elt.GetKind() != metadata.KindKeyValue {
			continue
		}
		keyArg := elt.X
		valArg := elt.Fun
		if keyArg == nil || valArg == nil {
			continue
		}
		if keyArg.GetKind() != metadata.KindIdent || valArg.GetKind() != metadata.KindIdent {
			continue
		}
		fieldName := keyArg.GetName()
		paramName := valArg.GetName()
		if fieldName == "" || paramName == "" {
			continue
		}
		if !params[paramName] {
			continue
		}
		out[fieldName] = paramName
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// resolveOverrideGoType produces the Go-type string for one bound
// field by (1) walking up via the parent helper-call's ParamArgMap
// when the constructor's arg is a helper-parameter passthrough, then
// (2) falling back to the constructor-arg's own declared type. The
// result is run through cleanOverrideType, which drops values that
// aren't real Go types (function names, untyped expressions,
// interface{}, …) so the override stays a safe no-op for those
// shapes rather than emitting a $ref to a non-existent component.
func (r *ResponsePatternMatcherImpl) resolveOverrideGoType(argAtCtor *metadata.CallArgument, parentEdge *metadata.CallGraphEdge) string {
	if argAtCtor == nil {
		return ""
	}
	if argAtCtor.GetKind() == metadata.KindIdent && parentEdge != nil && parentEdge.ParamArgMap != nil {
		if callerArg, ok := parentEdge.ParamArgMap[argAtCtor.GetName()]; ok {
			if t := cleanOverrideType(extractCallerArgType(&callerArg, r.contextProvider)); t != "" {
				return t
			}
		}
	}
	return cleanOverrideType(argAtCtor.GetType())
}

// extractCallerArgType pulls the Go-type string from a caller-side
// CallArgument. handleIdent (for variable refs) and handleCallExpr
// (for inline call expressions, single-valued) both populate
// arg.Type, so prefer it directly. Falls back to the stringified
// form, which for KindIdent gives the qualified type and for
// KindCall gives the function name (filtered out by
// cleanOverrideType).
func extractCallerArgType(arg *metadata.CallArgument, cp ContextProvider) string {
	if t := arg.GetType(); t != "" {
		return t
	}
	return cp.GetArgumentInfo(arg)
}

// cleanOverrideType normalises a Go-type string for use by the
// schema mapper, returning "" when the input doesn't describe a
// concrete user-defined type. Rejected shapes:
//
//   - empty / interface{} / any — no information beyond the
//     wrapper's declared field type.
//   - "untyped …" — Go's type-system tag for untyped constants.
//   - bare identifiers with no dot, slash, or container prefix that
//     aren't recognised primitives — almost always a function name
//     (e.g. `mapToGeneric`) that leaked through GetArgumentInfo for a
//     KindCall whose return type wasn't populated.
func cleanOverrideType(t string) string {
	t = strings.TrimPrefix(strings.TrimSpace(t), "&")
	t = strings.TrimPrefix(t, "*")
	if t == "" || t == "interface{}" || t == "any" {
		return ""
	}
	if strings.HasPrefix(t, "untyped ") {
		return ""
	}
	if !strings.ContainsAny(t, "./[") && !metadata.IsPrimitiveType(t) {
		return ""
	}
	return t
}

// specialiseWrapperSchema composes a per-route response schema by
// taking the base wrapper $ref and overlaying an inline object that
// overrides the resolved fields. Result shape:
//
//	allOf:
//	  - $ref: '#/components/schemas/Envelope'
//	  - type: object
//	    properties:
//	      data:
//	        $ref: '#/components/schemas/Order'
//
// If baseSchema isn't a $ref (e.g. the mapper inlined it) or no
// override property survived JSON-name resolution, the original
// schema is returned unchanged.
func specialiseWrapperSchema(baseSchema *Schema, overrides []wrapperFieldOverride, wrapperGoType string, usedTypes map[string]*Schema, meta *metadata.Metadata, cfg *APISpecConfig) *Schema {
	if baseSchema == nil || baseSchema.Ref == "" || len(overrides) == 0 {
		return baseSchema
	}
	wrapperType := lookupWrapperType(meta, wrapperGoType)
	if wrapperType == nil {
		return baseSchema
	}

	properties := map[string]*Schema{}
	for _, override := range overrides {
		// Only specialise fields whose declared wrapper type is
		// genuinely generic (interface{} / any). Fields with a
		// concrete declared type — e.g. `Message string`, `Code int`
		// — already render correctly from the base $ref, and
		// overriding them would mis-render the call-site literal
		// (e.g. http.StatusOK or "ok") as the field's type.
		if !wrapperFieldIsGeneric(meta, wrapperType, override.StructFieldName) {
			continue
		}
		jsonName := jsonNameForField(meta, wrapperType, override.StructFieldName)
		if jsonName == "" {
			continue
		}
		propSchema, discovered := mapGoTypeToOpenAPISchema(usedTypes, override.GoType, meta, cfg, nil)
		if propSchema == nil {
			continue
		}
		// mapGoTypeToOpenAPISchema returns the payload's $ref in
		// propSchema but hands its freshly discovered component
		// definitions (and any unresolved-external placeholder) back in
		// `discovered` — the caller is responsible for registering
		// them. Every other call site folds these into the type set
		// that becomes components/schemas; if we drop them, the `data`
		// $ref we just produced can point at a component nothing ever
		// populates (e.g. when the payload type lives outside the
		// analysed module), which Redoc rejects as an invalid
		// reference. markUsedType registers the name so generateSchemas
		// emits a definition (or, at worst, a placeholder) for it.
		for name, sch := range discovered {
			markUsedType(usedTypes, name, sch)
		}
		properties[jsonName] = propSchema
	}
	if len(properties) == 0 {
		return baseSchema
	}
	return &Schema{
		AllOf: []*Schema{
			baseSchema,
			{Type: "object", Properties: properties},
		},
	}
}

// --- helpers ---------------------------------------------------------

func metadataFromContextProvider(cp ContextProvider) *metadata.Metadata {
	if impl, ok := cp.(*ContextProviderImpl); ok {
		return impl.meta
	}
	return nil
}

func findEnclosingFunction(meta *metadata.Metadata, pkgIdx, nameIdx int) *metadata.Function {
	return findFunction(meta, meta.StringPool.GetString(pkgIdx), meta.StringPool.GetString(nameIdx))
}

func paramNamesOf(fn *metadata.Function) []string {
	if fn == nil {
		return nil
	}
	out := make([]string, 0, len(fn.Signature.Args))
	for _, p := range fn.Signature.Args {
		if p == nil {
			out = append(out, "")
			continue
		}
		out = append(out, p.GetName())
	}
	return out
}

func paramNameSetOf(fn *metadata.Function) map[string]bool {
	names := paramNamesOf(fn)
	out := make(map[string]bool, len(names))
	for _, n := range names {
		if n != "" {
			out[n] = true
		}
	}
	return out
}

func lookupCallArgByParamName(callArgs []*metadata.CallArgument, paramNames []string, name string) *metadata.CallArgument {
	for i, pn := range paramNames {
		if pn == name && i < len(callArgs) {
			return callArgs[i]
		}
	}
	return nil
}

func parentEdgeOf(node TrackerNodeInterface) *metadata.CallGraphEdge {
	parent := node.GetParent()
	if parent == nil {
		return nil
	}
	return parent.GetEdge()
}

func lookupWrapperType(meta *metadata.Metadata, goType string) *metadata.Type {
	if meta == nil || goType == "" {
		return nil
	}
	goType = strings.TrimPrefix(goType, "*")
	parts := TypeParts(goType)
	if parts.TypeName == "" {
		return nil
	}
	return typeByName(parts, meta)
}

// wrapperFieldIsGeneric reports whether the declared type of the named
// struct field on wrapperType is `interface{}` or `any` — i.e. the
// type system carries no concrete information and a per-route override
// is meaningful. Fields with concrete declared types (string, int,
// named structs, …) shouldn't be overridden by call-site literals.
func wrapperFieldIsGeneric(meta *metadata.Metadata, wrapperType *metadata.Type, structFieldName string) bool {
	if wrapperType == nil {
		return false
	}
	for _, field := range wrapperType.Fields {
		if meta.StringPool.GetString(field.Name) != structFieldName {
			continue
		}
		declared := meta.StringPool.GetString(field.Type)
		declared = strings.TrimPrefix(declared, "*")
		return declared == "interface{}" || declared == "any"
	}
	return false
}

func jsonNameForField(meta *metadata.Metadata, wrapperType *metadata.Type, structFieldName string) string {
	if wrapperType == nil {
		return ""
	}
	for _, field := range wrapperType.Fields {
		if meta.StringPool.GetString(field.Name) != structFieldName {
			continue
		}
		tag := meta.StringPool.GetString(field.Tag)
		if name := extractJSONName(tag); name != "" {
			return name
		}
		return structFieldName
	}
	return ""
}
