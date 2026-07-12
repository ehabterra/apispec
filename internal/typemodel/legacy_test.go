package typemodel

import (
	"reflect"
	"testing"
)

// TestParseParts pins the transitional view to the exact behavior of the
// spec layer's original TypeParts, including the quirks the port preserves.
// The first block mirrors the table in internal/spec's
// TestTypeParts_Comprehensive; the second block pins the quirks explicitly.
func TestParseParts(t *testing.T) {
	tests := []struct {
		input string
		want  Parts
	}{
		{"string", Parts{TypeName: "string"}},
		{"main-->User", Parts{PkgName: "main", TypeName: "User"}},
		{"pkg-->Type-->T", Parts{PkgName: "pkg", TypeName: "Type", GenericTypes: []string{"T"}}},
		{"Container[T]", Parts{TypeName: "Container[T]"}},
		{"Container[T, U]", Parts{TypeName: "Container[T, U]"}},
		{"pkg-->Container[T]", Parts{PkgName: "pkg", TypeName: "Container", GenericTypes: []string{"T"}}},
		{"pkg-->Pair[K, V]", Parts{PkgName: "pkg", TypeName: "Pair", GenericTypes: []string{"K", "V"}}},
		{"pkg-->Box[Page[User]]", Parts{PkgName: "pkg", TypeName: "Box", GenericTypes: []string{"Page[User]"}}},
		{"pkg.Envelope[pkg.User]", Parts{PkgName: "pkg", TypeName: "Envelope", GenericTypes: []string{"pkg.User"}}},
		{"github.com/x/y.Page[github.com/x/y.User]", Parts{PkgName: "github.com/x/y", TypeName: "Page", GenericTypes: []string{"github.com/x/y.User"}}},

		// Quirks preserved by the port (see the ParseParts doc comment):
		// one leading wrapper marker is stripped from the package only.
		{"*pkg-->User", Parts{PkgName: "pkg", TypeName: "User"}},
		{"[]pkg.User", Parts{PkgName: "pkg", TypeName: "User"}},
		// map syntax is not understood; the key leaks into PkgName.
		{"map[string]pkg.User", Parts{PkgName: "map[string]pkg", TypeName: "User"}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := ParseParts(tt.input); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseParts(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

// TestNormalizeInstance mirrors internal/spec's TestNormalizeGenericInstanceName.
func TestNormalizeInstance(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"pkg.Envelope[pkg.Product]", "pkg-->Envelope[Product]"},
		{"m/x.Page[m/x.User]", "m/x-->Page[User]"},
		{"pkg.Pair[pkg.User, pkg.Product]", "pkg-->Pair[User, Product]"},
		{"pkg.Envelope[pkg.Page[pkg.User]]", "pkg-->Envelope[Page[User]]"},
		{"Page[github.com/acme/svc.User]", "github.com/acme/svc-->Page[User]"},
		{"Page[pkg.User]", "pkg-->Page[User]"},
		{"pkg-->Envelope[User]", "pkg-->Envelope[User]"},
		{"pkg.User", "pkg.User"},
		{"Container[T]", "Container[T]"},
		{"string", "string"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := NormalizeInstance(tt.input); got != tt.want {
				t.Errorf("NormalizeInstance(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestSimplifyArgAndSimpleName pins the leaf-name reductions, including the
// legacy marker-dropping quirk that the structured Simple() does not share.
func TestSimplifyArgAndSimpleName(t *testing.T) {
	simplify := []struct{ input, want string }{
		{"pkg.User", "User"},
		{"pkg-->User", "User"},
		{"pkg.Page[pkg.User]", "Page[User]"},
		{"pkg.Pair[pkg.User, pkg.Product]", "Pair[User, Product]"},
		{"[]pkg.User", "User"}, // quirk: wrapper markers are dropped
		{"*pkg.User", "User"},  // quirk: wrapper markers are dropped
	}
	for _, tt := range simplify {
		if got := SimplifyArg(tt.input); got != tt.want {
			t.Errorf("SimplifyArg(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}

	simple := []struct{ input, want string }{
		{"github.com/x/y.User", "User"},
		{"pkg-->User", "User"},
		{"interface{}", "any"},
		{"interface {}", "any"},
		{"T any", "T any"}, // declaration form passes through
		{"User", "User"},
	}
	for _, tt := range simple {
		if got := SimpleName(tt.input); got != tt.want {
			t.Errorf("SimpleName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestArgPackage pins qualifier extraction.
func TestArgPackage(t *testing.T) {
	tests := []struct{ input, want string }{
		{"github.com/acme/svc.User", "github.com/acme/svc"},
		{"pkg-->User", "pkg"},
		{"pkg.Page[pkg.User]", "pkg"},
		{"[]pkg.User", "pkg"},
		{"User", ""},
		{"T any", ""},
	}
	for _, tt := range tests {
		if got := ArgPackage(tt.input); got != tt.want {
			t.Errorf("ArgPackage(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestSplitArgs pins top-level comma splitting.
func TestSplitArgs(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"K,V", []string{"K", "V"}},
		{"T any, U comparable", []string{"T any", "U comparable"}},
		{"Page[User], Box[Pair[K, V]]", []string{"Page[User]", "Box[Pair[K, V]]"}},
		{"  User  ", []string{"User"}},
	}
	for _, tt := range tests {
		if got := SplitArgs(tt.input); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("SplitArgs(%q) = %#v, want %#v", tt.input, got, tt.want)
		}
	}
}
