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
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
)

// packageValueField resolves `pkg.Config.Key` — a field read off a
// PACKAGE-LEVEL struct value — to what the declaration puts there.
//
// structFieldValue handles the composite literal that is in scope at the call
// site, which is what a generated server's base URL needs. A package-level
// value is not in scope anywhere: the literal sits in its declaration, in
// another file and usually another package, so that rung found no composite and
// the name was dropped. Every other normal spelling of a parameter name already
// resolved, which left this the one shape `testdata/param_name_sources` still
// recorded as unresolved (issue #455).
//
// Nothing about the literal is parsed. Metadata already records it structurally
// — a StructInstance with the field values — and the declaration is found by
// POSITION rather than by type name, which matters twice over: a package can
// declare several values of one type, and the variable's own recorded Type is
// not reliable for an inferred declaration (`var Config = Settings{…}` records
// no type at all).
func (b *BasePatternMatcher) packageValueField(arg *metadata.CallArgument, field string) (string, bool) {
	impl, ok := b.contextProvider.(*ContextProviderImpl)
	if !ok || impl.meta == nil || field == "" {
		return "", false
	}
	if arg == nil || arg.GetKind() != metadata.KindSelector || arg.X == nil || arg.Sel == nil {
		return "", false
	}

	pkgKey := selectorPackageKey(arg.X)
	pkg, exists := impl.meta.Packages[pkgKey]
	if !exists || pkg == nil {
		return "", false
	}
	valueName := arg.Sel.GetName()
	if valueName == "" {
		return "", false
	}

	// Sorted, because a name declared in two files of one package would
	// otherwise resolve differently between runs and this value reaches the
	// output as a parameter's name (golden rule #1).
	for _, fileName := range impl.meta.SortedFileNames(pkgKey) {
		file := pkg.Files[fileName]
		if file == nil {
			continue
		}
		variable, ok := file.Variables[valueName]
		if !ok || variable == nil {
			continue
		}
		// Only a composite literal has fields to read. A value built by a call
		// or copied from elsewhere does not settle them here, and guessing is
		// what issue #452 was about.
		if impl.GetString(variable.ValueKind) != metadata.KindCompositeLit {
			return "", false
		}
		return fieldOfInstanceAt(impl, file, impl.GetString(variable.Position), field)
	}
	return "", false
}

// fieldOfInstanceAt reads one field from the struct literal declared at pos.
//
// The literal is matched by position rather than by type: `var A, B = T{…},
// T{…}` declares two instances of one type, and a package that declares several
// values of a type is ordinary. A declaration and its literal share a line —
// the literal starts just after the name — so the line is the key, and anything
// but exactly one match on it is declined rather than chosen between
// (golden rule #7).
func fieldOfInstanceAt(impl *ContextProviderImpl, file *metadata.File, pos, field string) (string, bool) {
	line, ok := fileLine(pos)
	if !ok {
		return "", false
	}
	var found *metadata.StructInstance
	for i := range file.StructInstances {
		instance := &file.StructInstances[i]
		instLine, ok := fileLine(impl.GetString(instance.Position))
		if !ok || instLine != line {
			continue
		}
		if found != nil {
			return "", false // two literals on one line: which one is not stated
		}
		found = instance
	}
	if found == nil {
		return "", false
	}

	for nameIdx, valueIdx := range found.Fields {
		if impl.GetString(nameIdx) != field {
			continue
		}
		// The field's VALUE, not the expression it was written from.
		// Fields holds those rendered, so `Settings{Key: compute()}` reads back
		// as `func() string()` — a header name no client can send, and the same
		// defect #452 fixed for a variable's initializer.
		if impl.GetString(found.FieldKinds[nameIdx]) != metadata.KindLiteral {
			return "", false
		}
		value := impl.GetString(valueIdx)
		if value == "" {
			return "", false
		}
		return unquoteLiteral(value), true
	}
	// The literal does not set this field, so it holds the zero value — which
	// for the string a parameter name is means there is no name here. Reported
	// as resolved-to-nothing rather than unresolved, matching structFieldValue:
	// the caller drops a parameter with an empty name either way (#452).
	return "", true
}

// fileLine is the "file:line" prefix of a recorded position, dropping the
// column. A declaration and the literal it initialises start on the same line
// at different columns.
func fileLine(pos string) (string, bool) {
	if pos == "" {
		return "", false
	}
	last := strings.LastIndexByte(pos, ':')
	if last <= 0 {
		return "", false
	}
	prefix := pos[:last]
	if strings.LastIndexByte(prefix, ':') <= 0 {
		return "", false // no line component: a bare filename
	}
	return prefix, true
}
