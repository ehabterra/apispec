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

package core

import "sort"

// StdlibFramework is the name reported when no third-party framework import is
// found. net/http is imported by nearly every Go project and carries no routing
// signal on its own, so it is never *detected* — only fallen back to.
const StdlibFramework = "net/http"

// Framework describes one web framework apispec knows about. It is the single
// source for every list that used to be written out per consumer: the
// source-import scan (internal/core), the dependency analyser's patterns,
// external prefixes and detection order (internal/metadata), and the framework
// picker in the UI. Before this registry existed the set was restated six
// times, including as a bare `const knownFrameworks = 5` bounding the file walk
// — adding a sixth framework truncated detection silently (issue #285).
type Framework struct {
	// Name is the canonical name used by the CLI, the engine and the UI.
	Name string

	// DependencyKey is the key the metadata dependency analyser files this
	// framework under. It matches Name except for net/http, whose bucket has
	// always been "http" and appears in emitted metadata.
	DependencyKey string

	// SourcePatterns are import-path substrings the source-level scan
	// (FrameworkDetector.DetectAll) matches. Empty means the framework is not
	// source-detectable: net/http carries no routing signal, and fasthttp is
	// only classified during dependency analysis.
	SourcePatterns []string

	// ImportPatterns are import-path prefixes that identify a package as using
	// this framework during dependency analysis.
	ImportPatterns []string

	// ExternalPrefixes are import-path prefixes excluded as external (vendor)
	// dependencies. Deliberately not derived from ImportPatterns: companion
	// modules such as github.com/gin-contrib/ identify a framework user without
	// being excluded from analysis.
	ExternalPrefixes []string

	// DetectionRank orders dependency-analysis classification for packages that
	// import more than one framework (nearly all handler code also imports
	// net/http). Lower wins; the stdlib bucket is always considered last.
	DetectionRank int

	// HasDefaultConfig reports whether internal/spec ships a default pattern
	// config for this framework, i.e. whether it can be selected as the primary
	// framework. spec.DefaultConfigForFramework is the lookup; a test there
	// asserts the two stay in step.
	HasDefaultConfig bool
}

// SourceDetectable reports whether the source-import scan can find f.
func (f Framework) SourceDetectable() bool { return len(f.SourcePatterns) > 0 }

// frameworks is the registry. Declaration order is the order the source scan
// and the UI picker present frameworks in; classification order during
// dependency analysis is DetectionRank, which differs by history and is kept
// explicit so neither ordering silently changes when an entry is added.
var frameworks = []Framework{
	{
		Name:             "gin",
		DependencyKey:    "gin",
		SourcePatterns:   []string{"gin-gonic/gin"},
		ImportPatterns:   []string{"github.com/gin-gonic/gin", "github.com/gin-contrib/"},
		ExternalPrefixes: []string{"github.com/gin-gonic/gin"},
		DetectionRank:    1,
		HasDefaultConfig: true,
	},
	{
		Name:             "chi",
		DependencyKey:    "chi",
		SourcePatterns:   []string{"go-chi/chi"},
		ImportPatterns:   []string{"github.com/go-chi/chi", "github.com/go-chi/chi/v5"},
		ExternalPrefixes: []string{"github.com/go-chi/chi"},
		DetectionRank:    4,
		HasDefaultConfig: true,
	},
	{
		Name:             "echo",
		DependencyKey:    "echo",
		SourcePatterns:   []string{"labstack/echo"},
		ImportPatterns:   []string{"github.com/labstack/echo", "github.com/labstack/echo/v4"},
		ExternalPrefixes: []string{"github.com/labstack/echo"},
		DetectionRank:    2,
		HasDefaultConfig: true,
	},
	{
		Name:             "fiber",
		DependencyKey:    "fiber",
		SourcePatterns:   []string{"gofiber/fiber"},
		ImportPatterns:   []string{"github.com/gofiber/fiber", "github.com/gofiber/fiber/v2"},
		ExternalPrefixes: []string{"github.com/gofiber/fiber"},
		DetectionRank:    3,
		HasDefaultConfig: true,
	},
	{
		Name:             "mux",
		DependencyKey:    "mux",
		SourcePatterns:   []string{"gorilla/mux"},
		ImportPatterns:   []string{"github.com/gorilla/mux"},
		ExternalPrefixes: []string{"github.com/gorilla/mux"},
		DetectionRank:    5,
		HasDefaultConfig: true,
	},
	{
		// No route-extraction config yet: fasthttp is classified during
		// dependency analysis only, so a project using it is analysed with the
		// stdlib surface rather than being silently dropped.
		Name:             "fasthttp",
		DependencyKey:    "fasthttp",
		ImportPatterns:   []string{"github.com/valyala/fasthttp"},
		ExternalPrefixes: []string{"github.com/valyala/fasthttp"},
		DetectionRank:    6,
	},
	{
		Name:             StdlibFramework,
		DependencyKey:    "http",
		ImportPatterns:   []string{"net/http"},
		DetectionRank:    stdlibDetectionRank,
		HasDefaultConfig: true,
	},
}

// stdlibDetectionRank keeps the net/http bucket last: it is the one pattern
// that matches nearly every package, so any framework import must outrank it.
const stdlibDetectionRank = 1 << 30

// StdlibDependencyKey is the dependency-analysis bucket net/http is filed
// under. It is considered after every other framework: its pattern matches
// nearly every package, so a framework import must always win.
const StdlibDependencyKey = "http"

// FrameworksByDetectionRank returns the registry ordered by DetectionRank —
// the order a package importing several frameworks is classified in.
func FrameworksByDetectionRank() []Framework {
	out := Frameworks()
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].DetectionRank < out[j].DetectionRank
	})
	return out
}

// Frameworks returns the registry in declaration order.
func Frameworks() []Framework {
	out := make([]Framework, len(frameworks))
	copy(out, frameworks)
	return out
}

// SourceDetectableFrameworks returns the frameworks the source-import scan can
// find, in declaration order.
func SourceDetectableFrameworks() []Framework {
	out := make([]Framework, 0, len(frameworks))
	for _, f := range frameworks {
		if f.SourceDetectable() {
			out = append(out, f)
		}
	}
	return out
}

// ConfigurableFrameworkNames returns the names of frameworks that ship a
// default pattern config, in declaration order — the set the UI offers.
func ConfigurableFrameworkNames() []string {
	out := make([]string, 0, len(frameworks))
	for _, f := range frameworks {
		if f.HasDefaultConfig {
			out = append(out, f.Name)
		}
	}
	return out
}
