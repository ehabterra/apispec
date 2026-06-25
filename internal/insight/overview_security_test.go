package insight

import (
	"testing"

	"github.com/ehabterra/apispec/internal/spec"
)

func reqPtr(r ...spec.SecurityRequirement) *[]spec.SecurityRequirement {
	s := append([]spec.SecurityRequirement{}, r...)
	return &s
}

// TestSecurityStats covers the protected / public / no-auth classification,
// including OpenAPI's empty requirement object {} (anonymous access) and
// document-level inheritance.
func TestSecurityStats(t *testing.T) {
	s := &spec.OpenAPISpec{
		Components: &spec.Components{
			SecuritySchemes: map[string]spec.SecurityScheme{
				"bearerAuth": {Type: "http", Scheme: "bearer"},
			},
		},
		Paths: map[string]spec.PathItem{
			// protected: a real requirement
			"/a": {Get: &spec.Operation{Security: reqPtr(spec.SecurityRequirement{"bearerAuth": {}})}},
			// public: explicit security: [] (override / opt out)
			"/b": {Get: &spec.Operation{Security: reqPtr()}},
			// no auth: nothing declared, no global default
			"/c": {Get: &spec.Operation{}},
			// public: [{}] — empty requirement object permits anonymous access
			"/d": {Get: &spec.Operation{Security: reqPtr(spec.SecurityRequirement{})}},
			// public: [{}, {bearerAuth}] — anonymous allowed alongside bearer
			"/e": {Get: &spec.Operation{Security: reqPtr(spec.SecurityRequirement{}, spec.SecurityRequirement{"bearerAuth": {}})}},
		},
	}

	st := securityStats(s)

	if st.Protected != 1 {
		t.Errorf("Protected = %d, want 1", st.Protected)
	}
	if st.Public != 3 { // /b, /d, /e
		t.Errorf("Public = %d, want 3", st.Public)
	}
	if st.Unsecured != 1 { // /c
		t.Errorf("Unsecured = %d, want 1", st.Unsecured)
	}
	if st.SchemesDefined != 1 || len(st.Schemes) != 1 || st.Schemes[0] != "bearerAuth" {
		t.Errorf("schemes = %+v (defined %d), want [bearerAuth]", st.Schemes, st.SchemesDefined)
	}
	// Only the genuinely-protected /a contributes to scheme usage.
	if len(st.BySchemeUsage) != 1 || st.BySchemeUsage[0].Name != "bearerAuth" || st.BySchemeUsage[0].Count != 1 {
		t.Errorf("BySchemeUsage = %+v, want bearerAuth:1", st.BySchemeUsage)
	}
}

// TestSecurityStatsGlobalInheritance verifies that an operation with no security
// field inherits the document-level requirement.
func TestSecurityStatsGlobalInheritance(t *testing.T) {
	s := &spec.OpenAPISpec{
		Security: []spec.SecurityRequirement{{"bearerAuth": {}}},
		Paths: map[string]spec.PathItem{
			"/inherits": {Get: &spec.Operation{}},                   // inherits global -> protected
			"/optsout":  {Get: &spec.Operation{Security: reqPtr()}}, // security: [] -> public
		},
	}
	st := securityStats(s)
	if !st.GlobalSecurity {
		t.Error("GlobalSecurity = false, want true")
	}
	if st.Protected != 1 {
		t.Errorf("Protected = %d, want 1 (inherited)", st.Protected)
	}
	if st.Public != 1 {
		t.Errorf("Public = %d, want 1", st.Public)
	}
}
