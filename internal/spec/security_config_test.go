package spec

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// TestSecurityConfigYAMLRoundTrip verifies the new SecurityPattern /
// SecurityMapping config types marshal and unmarshal through YAML, including
// the AND (schemes) / OR (schemesAnyOf) / public shapes.
func TestSecurityConfigYAMLRoundTrip(t *testing.T) {
	in := &APISpecConfig{
		Framework: FrameworkConfig{
			SecurityPatterns: []SecurityPattern{
				{
					CallRegex:          "^Use$",
					RecvTypeRegex:      `chi\.Router`,
					Scope:              SecurityScopeRouter,
					MiddlewareArgIndex: 0,
					MiddlewareVariadic: true,
				},
				{
					CallRegex:       "^Group$",
					Scope:           SecurityScopeSubtree,
					HandlerArgIndex: 1,
				},
			},
		},
		SecurityMappings: []SecurityMapping{
			{
				FunctionNameRegex: "^authMiddleware$",
				RecvTypeRegex:     "Handler",
				Schemes:           []SecurityRequirement{{"bearerAuth": {}}},
			},
			{
				FunctionNameRegex: "^New$",
				PkgRegex:          `github\.com/golang-jwt/.*`,
				SchemesAnyOf: [][]SecurityRequirement{
					{{"bearerAuth": {}}},
					{{"apiKeyAuth": {}}},
				},
			},
			{
				FunctionNameRegex: "^AllowPublic$",
				Public:            true,
			},
		},
	}

	data, err := yaml.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out APISpecConfig
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got := len(out.Framework.SecurityPatterns); got != 2 {
		t.Fatalf("securityPatterns: got %d, want 2\nYAML:\n%s", got, data)
	}
	if sp := out.Framework.SecurityPatterns[0]; sp.Scope != SecurityScopeRouter || !sp.MiddlewareVariadic {
		t.Errorf("pattern[0] round-trip mismatch: %+v", sp)
	}
	if got := len(out.SecurityMappings); got != 3 {
		t.Fatalf("securityMappings: got %d, want 3", got)
	}
	if m := out.SecurityMappings[0]; len(m.Schemes) != 1 {
		t.Errorf("mapping[0] schemes round-trip mismatch: %+v", m)
	} else if _, ok := m.Schemes[0]["bearerAuth"]; !ok {
		t.Errorf("mapping[0] lost the bearerAuth scheme key: %+v", m.Schemes[0])
	}
	if m := out.SecurityMappings[1]; len(m.SchemesAnyOf) != 2 {
		t.Errorf("mapping[1] schemesAnyOf round-trip mismatch: %+v", m)
	}
	if m := out.SecurityMappings[2]; !m.Public {
		t.Errorf("mapping[2] public round-trip mismatch: %+v", m)
	}
}

func TestValidateSecurityConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     APISpecConfig
		wantErr bool
	}{
		{
			name: "empty config is valid",
			cfg:  APISpecConfig{},
		},
		{
			name: "valid pattern and mapping",
			cfg: APISpecConfig{
				Framework: FrameworkConfig{SecurityPatterns: []SecurityPattern{
					{CallRegex: "^Use$", Scope: SecurityScopeRouter},
				}},
				SecurityMappings: []SecurityMapping{
					{FunctionNameRegex: "^auth$", Schemes: []SecurityRequirement{{"bearerAuth": {}}}},
				},
			},
		},
		{
			name: "invalid scope",
			cfg: APISpecConfig{Framework: FrameworkConfig{SecurityPatterns: []SecurityPattern{
				{CallRegex: "^Use$", Scope: "everywhere"},
			}}},
			wantErr: true,
		},
		{
			name: "pattern with no matcher",
			cfg: APISpecConfig{Framework: FrameworkConfig{SecurityPatterns: []SecurityPattern{
				{Scope: SecurityScopeRouter},
			}}},
			wantErr: true,
		},
		{
			name: "pattern with bad regex",
			cfg: APISpecConfig{Framework: FrameworkConfig{SecurityPatterns: []SecurityPattern{
				{CallRegex: "(", Scope: SecurityScopeRouter},
			}}},
			wantErr: true,
		},
		{
			name: "mapping with no identity matcher",
			cfg: APISpecConfig{SecurityMappings: []SecurityMapping{
				{Schemes: []SecurityRequirement{{"bearerAuth": {}}}},
			}},
			wantErr: true,
		},
		{
			name: "mapping with no result",
			cfg: APISpecConfig{SecurityMappings: []SecurityMapping{
				{FunctionNameRegex: "^auth$"},
			}},
			wantErr: true,
		},
		{
			name: "public mapping needs no schemes",
			cfg: APISpecConfig{SecurityMappings: []SecurityMapping{
				{FunctionNameRegex: "^skip$", Public: true},
			}},
		},
		{
			name: "mapping with bad regex",
			cfg: APISpecConfig{SecurityMappings: []SecurityMapping{
				{PkgRegex: "[", Schemes: []SecurityRequirement{{"bearerAuth": {}}}},
			}},
			wantErr: true,
		},
		{
			name: "mapping with blank scheme key",
			cfg: APISpecConfig{SecurityMappings: []SecurityMapping{
				{FunctionNameRegex: "^auth$", Schemes: []SecurityRequirement{{"  ": {}}}},
			}},
			wantErr: true,
		},
		{
			name: "mapping with blank scheme key in schemesAnyOf",
			cfg: APISpecConfig{SecurityMappings: []SecurityMapping{
				{FunctionNameRegex: "^auth$", SchemesAnyOf: [][]SecurityRequirement{{{"": {}}}}},
			}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.ValidateSecurity()
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateSecurityConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
