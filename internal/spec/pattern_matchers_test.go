package spec

import (
	"testing"

	"github.com/ehabterra/swagen/internal/metadata"
)

func TestNewBasePatternMatcher(t *testing.T) {
	cfg := DefaultSwagenConfig()
	meta := &metadata.Metadata{}
	contextProvider := NewContextProvider(meta)
	typeResolver := NewTypeResolver(meta, cfg, NewSchemaMapper(cfg))

	matcher := NewBasePatternMatcher(cfg, contextProvider, typeResolver)
	if matcher == nil {
		t.Fatal("NewBasePatternMatcher returned nil")
	}

	if matcher.contextProvider == nil {
		t.Error("ContextProvider is nil")
	}

	if matcher.cfg == nil {
		t.Error("Config is nil")
	}

	if matcher.typeResolver == nil {
		t.Error("TypeResolver is nil")
	}
}

func TestNewRoutePatternMatcher(t *testing.T) {
	cfg := DefaultSwagenConfig()
	meta := &metadata.Metadata{}
	contextProvider := NewContextProvider(meta)
	typeResolver := NewTypeResolver(meta, cfg, NewSchemaMapper(cfg))

	pattern := RoutePattern{
		CallRegex:   "^GET$",
		PathFromArg: true,
	}

	matcher := NewRoutePatternMatcher(pattern, cfg, contextProvider, typeResolver)
	if matcher == nil {
		t.Fatal("NewRoutePatternMatcher returned nil")
	}

	if matcher.pattern.CallRegex != "^GET$" {
		t.Errorf("Expected CallRegex '^GET$', got '%s'", matcher.pattern.CallRegex)
	}
}

func TestNewMountPatternMatcher(t *testing.T) {
	cfg := DefaultSwagenConfig()
	meta := &metadata.Metadata{}
	contextProvider := NewContextProvider(meta)
	typeResolver := NewTypeResolver(meta, cfg, NewSchemaMapper(cfg))

	pattern := MountPattern{
		CallRegex:   "^Use$",
		PathFromArg: true,
	}

	matcher := NewMountPatternMatcher(pattern, cfg, contextProvider, typeResolver)
	if matcher == nil {
		t.Fatal("NewMountPatternMatcher returned nil")
	}

	if matcher.pattern.CallRegex != "^Use$" {
		t.Errorf("Expected CallRegex '^Use$', got '%s'", matcher.pattern.CallRegex)
	}
}

func TestNewRequestPatternMatcher(t *testing.T) {
	cfg := DefaultSwagenConfig()
	meta := &metadata.Metadata{}
	contextProvider := NewContextProvider(meta)
	typeResolver := NewTypeResolver(meta, cfg, NewSchemaMapper(cfg))

	pattern := RequestBodyPattern{
		CallRegex:   "^Bind$",
		TypeFromArg: true,
	}

	matcher := NewRequestPatternMatcher(pattern, cfg, contextProvider, typeResolver)
	if matcher == nil {
		t.Fatal("NewRequestPatternMatcher returned nil")
	}

	if matcher.pattern.CallRegex != "^Bind$" {
		t.Errorf("Expected CallRegex '^Bind$', got '%s'", matcher.pattern.CallRegex)
	}
}

func TestNewResponsePatternMatcher(t *testing.T) {
	cfg := DefaultSwagenConfig()
	meta := &metadata.Metadata{}
	contextProvider := NewContextProvider(meta)
	typeResolver := NewTypeResolver(meta, cfg, NewSchemaMapper(cfg))

	pattern := ResponsePattern{
		CallRegex:   "^JSON$",
		TypeFromArg: true,
	}

	matcher := NewResponsePatternMatcher(pattern, cfg, contextProvider, typeResolver)
	if matcher == nil {
		t.Fatal("NewResponsePatternMatcher returned nil")
	}

	if matcher.pattern.CallRegex != "^JSON$" {
		t.Errorf("Expected CallRegex '^JSON$', got '%s'", matcher.pattern.CallRegex)
	}
}

func TestNewParamPatternMatcher(t *testing.T) {
	cfg := DefaultSwagenConfig()
	meta := &metadata.Metadata{}
	contextProvider := NewContextProvider(meta)
	typeResolver := NewTypeResolver(meta, cfg, NewSchemaMapper(cfg))

	pattern := ParamPattern{
		CallRegex: "^Param$",
		ParamIn:   "path",
	}

	matcher := NewParamPatternMatcher(pattern, cfg, contextProvider, typeResolver)
	if matcher == nil {
		t.Fatal("NewParamPatternMatcher returned nil")
	}

	if matcher.pattern.CallRegex != "^Param$" {
		t.Errorf("Expected CallRegex '^Param$', got '%s'", matcher.pattern.CallRegex)
	}
}

func TestRoutePatternMatcher_GetPattern(t *testing.T) {
	cfg := DefaultSwagenConfig()
	meta := &metadata.Metadata{}
	contextProvider := NewContextProvider(meta)
	typeResolver := NewTypeResolver(meta, cfg, NewSchemaMapper(cfg))

	pattern := RoutePattern{
		CallRegex:   "^GET$",
		PathFromArg: true,
	}

	matcher := NewRoutePatternMatcher(pattern, cfg, contextProvider, typeResolver)
	retrievedPattern := matcher.GetPattern()

	if retrievedPattern == nil {
		t.Fatal("GetPattern returned nil")
	}

	routePattern, ok := retrievedPattern.(RoutePattern)
	if !ok {
		t.Fatal("GetPattern did not return RoutePattern")
	}

	if routePattern.CallRegex != "^GET$" {
		t.Errorf("Expected CallRegex '^GET$', got '%s'", routePattern.CallRegex)
	}
}

func TestRoutePatternMatcher_GetPriority(t *testing.T) {
	cfg := DefaultSwagenConfig()
	meta := &metadata.Metadata{}
	contextProvider := NewContextProvider(meta)
	typeResolver := NewTypeResolver(meta, cfg, NewSchemaMapper(cfg))

	pattern := RoutePattern{
		CallRegex:   "^GET$",
		PathFromArg: true,
	}

	matcher := NewRoutePatternMatcher(pattern, cfg, contextProvider, typeResolver)
	priority := matcher.GetPriority()

	// Should have priority 10 for CallRegex
	if priority != 10 {
		t.Errorf("Expected priority 10, got %d", priority)
	}
}

func TestMountPatternMatcher_GetPattern(t *testing.T) {
	cfg := DefaultSwagenConfig()
	meta := &metadata.Metadata{}
	contextProvider := NewContextProvider(meta)
	typeResolver := NewTypeResolver(meta, cfg, NewSchemaMapper(cfg))

	pattern := MountPattern{
		CallRegex:   "^Use$",
		PathFromArg: true,
	}

	matcher := NewMountPatternMatcher(pattern, cfg, contextProvider, typeResolver)
	retrievedPattern := matcher.GetPattern()

	if retrievedPattern == nil {
		t.Fatal("GetPattern returned nil")
	}

	mountPattern, ok := retrievedPattern.(MountPattern)
	if !ok {
		t.Fatal("GetPattern did not return MountPattern")
	}

	if mountPattern.CallRegex != "^Use$" {
		t.Errorf("Expected CallRegex '^Use$', got '%s'", mountPattern.CallRegex)
	}
}

func TestMountPatternMatcher_GetPriority(t *testing.T) {
	cfg := DefaultSwagenConfig()
	meta := &metadata.Metadata{}
	contextProvider := NewContextProvider(meta)
	typeResolver := NewTypeResolver(meta, cfg, NewSchemaMapper(cfg))

	pattern := MountPattern{
		CallRegex:   "^Use$",
		PathFromArg: true,
	}

	matcher := NewMountPatternMatcher(pattern, cfg, contextProvider, typeResolver)
	priority := matcher.GetPriority()

	// Should have priority 10 for CallRegex
	if priority != 10 {
		t.Errorf("Expected priority 10, got %d", priority)
	}
}

func TestRequestPatternMatcher_GetPattern(t *testing.T) {
	cfg := DefaultSwagenConfig()
	meta := &metadata.Metadata{}
	contextProvider := NewContextProvider(meta)
	typeResolver := NewTypeResolver(meta, cfg, NewSchemaMapper(cfg))

	pattern := RequestBodyPattern{
		CallRegex:   "^Bind$",
		TypeFromArg: true,
	}

	matcher := NewRequestPatternMatcher(pattern, cfg, contextProvider, typeResolver)
	retrievedPattern := matcher.GetPattern()

	if retrievedPattern == nil {
		t.Fatal("GetPattern returned nil")
	}

	requestPattern, ok := retrievedPattern.(RequestBodyPattern)
	if !ok {
		t.Fatal("GetPattern did not return RequestBodyPattern")
	}

	if requestPattern.CallRegex != "^Bind$" {
		t.Errorf("Expected CallRegex '^Bind$', got '%s'", requestPattern.CallRegex)
	}
}

func TestResponsePatternMatcher_GetPattern(t *testing.T) {
	cfg := DefaultSwagenConfig()
	meta := &metadata.Metadata{}
	contextProvider := NewContextProvider(meta)
	typeResolver := NewTypeResolver(meta, cfg, NewSchemaMapper(cfg))

	pattern := ResponsePattern{
		CallRegex:   "^JSON$",
		TypeFromArg: true,
	}

	matcher := NewResponsePatternMatcher(pattern, cfg, contextProvider, typeResolver)
	retrievedPattern := matcher.GetPattern()

	if retrievedPattern == nil {
		t.Fatal("GetPattern returned nil")
	}

	responsePattern, ok := retrievedPattern.(ResponsePattern)
	if !ok {
		t.Fatal("GetPattern did not return ResponsePattern")
	}

	if responsePattern.CallRegex != "^JSON$" {
		t.Errorf("Expected CallRegex '^JSON$', got '%s'", responsePattern.CallRegex)
	}
}

func TestParamPatternMatcher_GetPattern(t *testing.T) {
	cfg := DefaultSwagenConfig()
	meta := &metadata.Metadata{}
	contextProvider := NewContextProvider(meta)
	typeResolver := NewTypeResolver(meta, cfg, NewSchemaMapper(cfg))

	pattern := ParamPattern{
		CallRegex: "^Param$",
		ParamIn:   "path",
	}

	matcher := NewParamPatternMatcher(pattern, cfg, contextProvider, typeResolver)
	retrievedPattern := matcher.GetPattern()

	if retrievedPattern == nil {
		t.Fatal("GetPattern returned nil")
	}

	paramPattern, ok := retrievedPattern.(ParamPattern)
	if !ok {
		t.Fatal("GetPattern did not return ParamPattern")
	}

	if paramPattern.CallRegex != "^Param$" {
		t.Errorf("Expected CallRegex '^Param$', got '%s'", paramPattern.CallRegex)
	}
}
