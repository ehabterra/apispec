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

package metadata

import "github.com/ehabterra/apispec/internal/typemodel"

// TypeRefOf returns the structured type reference for a pooled type string,
// parsing each distinct pool entry at most once. This is how metadata records
// "carry" their types structurally (phase 4 of docs/TYPE_MODEL.md) while the
// string pool remains the serialization format.
//
// The returned ref is shared across callers and MUST be treated as
// immutable — Clone() it before mutating.
func (m *Metadata) TypeRefOf(id int) *typemodel.TypeRef {
	if m == nil || m.StringPool == nil || id < 0 {
		return nil
	}

	m.cacheMutex.RLock()
	ref, ok := m.typeRefCache[id]
	m.cacheMutex.RUnlock()
	if ok {
		return ref
	}

	ref = typemodel.Parse(m.StringPool.GetString(id))

	m.cacheMutex.Lock()
	if m.typeRefCache == nil {
		m.typeRefCache = map[int]*typemodel.TypeRef{}
	}
	// Another goroutine may have won the race; keep its entry so every caller
	// observes one shared ref per pool id.
	if existing, ok := m.typeRefCache[id]; ok {
		ref = existing
	} else {
		m.typeRefCache[id] = ref
	}
	m.cacheMutex.Unlock()

	return ref
}

// TypeRef returns the structured reference of the argument's static type
// (the pooled Type field). Shared and immutable — see Metadata.TypeRefOf.
func (a *CallArgument) TypeRef() *typemodel.TypeRef {
	if a == nil || a.Meta == nil {
		return nil
	}
	return a.Meta.TypeRefOf(a.Type)
}

// ResolvedTypeRef returns the structured reference of the argument's resolved
// concrete type, or nil when no resolution was recorded. Shared and
// immutable — see Metadata.TypeRefOf.
func (a *CallArgument) ResolvedTypeRef() *typemodel.TypeRef {
	if a == nil || a.Meta == nil || a.ResolvedType <= 0 {
		return nil
	}
	return a.Meta.TypeRefOf(a.ResolvedType)
}
