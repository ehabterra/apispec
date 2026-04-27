// Copyright 2025 Ehab Terra
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package main

import (
	"bufio"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ehabterra/apispec/internal/core"
	"github.com/ehabterra/apispec/internal/engine"
	"github.com/ehabterra/apispec/internal/metadata"
	"github.com/ehabterra/apispec/internal/spec"
	pubspec "github.com/ehabterra/apispec/spec"
	"gopkg.in/yaml.v3"
)

//go:embed assets/*.html
var assets embed.FS

// supportedFrameworks lists frameworks the UI can pick from.
var supportedFrameworks = []string{"gin", "chi", "echo", "fiber", "mux", "net/http"}

// ServerConfig is the runtime config of the apispecui server.
type ServerConfig struct {
	Host       string
	Port       int
	InputDir   string
	ConfigFile string
	Verbose    bool
}

// DetectResponse is what GET /api/detect returns: information the UI needs
// to pre-fill the configuration form.
type DetectResponse struct {
	InputDir            string                         `json:"inputDir"`
	ModuleRoot          string                         `json:"moduleRoot"`
	ModulePath          string                         `json:"modulePath"`
	DetectedFramework   string                         `json:"detectedFramework"`
	SupportedFrameworks []string                       `json:"supportedFrameworks"`
	OpenAPIVersion      string                         `json:"openapiVersion"`
	Info                spec.Info                      `json:"info"`
	Servers             []spec.Server                  `json:"servers"`
	Security            []spec.SecurityRequirement     `json:"security"`
	SecuritySchemes     map[string]spec.SecurityScheme `json:"securitySchemes"`
	Tags                []spec.Tag                     `json:"tags"`
	ExternalDocs        *spec.ExternalDocumentation    `json:"externalDocs"`
	Defaults            spec.Defaults                  `json:"defaults"`
	TypeMapping         []spec.TypeMapping             `json:"typeMapping"`
	ExternalTypes       []spec.ExternalType            `json:"externalTypes"`
	Overrides           []spec.Override                `json:"overrides"`
	Include             spec.IncludeExclude            `json:"include"`
	Exclude             spec.IncludeExclude            `json:"exclude"`
	// Framework is the full default FrameworkConfig (route/requestBody/response/
	// param/mount patterns) for the detected framework — pre-filled so the UI
	// can render every pattern editor.
	Framework spec.FrameworkConfig `json:"frameworkConfig"`
}

// ProjectRequest is what POST /api/project accepts.
type ProjectRequest struct {
	Dir string `json:"dir"`
}

// GenerateRequest is what the UI POSTs to /api/generate.
//
// Two modes:
//  1. Structured: framework defaults + form fields are merged into APISpecConfig.
//  2. Raw: when UseRawConfig is true, RawConfig (YAML text) is parsed directly
//     into APISpecConfig and used as-is. Use this to edit any field — including
//     framework patterns, method extraction rules, etc. — that the form doesn't
//     surface.
type GenerateRequest struct {
	Framework       string                         `json:"framework"`
	OpenAPIVersion  string                         `json:"openapiVersion"`
	Info            spec.Info                      `json:"info"`
	Servers         []spec.Server                  `json:"servers"`
	Security        []spec.SecurityRequirement     `json:"security"`
	SecuritySchemes map[string]spec.SecurityScheme `json:"securitySchemes"`
	Tags            []spec.Tag                     `json:"tags"`
	ExternalDocs    *spec.ExternalDocumentation    `json:"externalDocs"`
	Defaults        spec.Defaults                  `json:"defaults"`
	TypeMapping     []spec.TypeMapping             `json:"typeMapping"`
	ExternalTypes   []spec.ExternalType            `json:"externalTypes"`
	Overrides       []spec.Override                `json:"overrides"`
	Include         spec.IncludeExclude            `json:"include"`
	Exclude         spec.IncludeExclude            `json:"exclude"`

	// FrameworkConfig replaces the named framework's default extraction
	// patterns when set. Used by the per-pattern editors in the UI (Routes /
	// Request Body / Responses / Parameters / Groups). Empty slices fall back
	// to the framework defaults.
	FrameworkConfig *spec.FrameworkConfig `json:"frameworkConfig"`

	// Raw-YAML escape hatch for fields the structured form does not expose
	// or for users that just want to author the full APISpecConfig directly.
	UseRawConfig bool   `json:"useRawConfig"`
	RawConfig    string `json:"rawConfig"`
}

// GenerateResponse is the result of a successful generation.
type GenerateResponse struct {
	OK          bool      `json:"ok"`
	Framework   string    `json:"framework"`
	PathCount   int       `json:"pathCount"`
	GeneratedAt time.Time `json:"generatedAt"`
	DurationMs  int64     `json:"durationMs"`
	Message     string    `json:"message,omitempty"`
}

// UIServer holds shared state across requests.
type UIServer struct {
	cfg *ServerConfig

	mu          sync.RWMutex
	inputDir    string // current project directory; mutable from the UI
	currentSpec *pubspec.OpenAPISpec
	currentCfg  *spec.APISpecConfig
	lastGen     time.Time
	lastErr     string

	// metaCache is keyed by absolute input dir. Cleared on project switch.
	metaCache map[string]*MetadataSummary
}

func (s *UIServer) currentDir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.inputDir
}

func (s *UIServer) setDir(d string) {
	s.mu.Lock()
	s.inputDir = d
	s.metaCache = nil // invalidate metadata cache
	s.mu.Unlock()
}

func main() {
	cfg := parseFlags()

	srv := &UIServer{cfg: cfg, inputDir: cfg.InputDir}

	mux := http.NewServeMux()
	mux.HandleFunc("/", srv.handleIndex)
	mux.HandleFunc("/swagger", srv.handleSwagger)
	mux.HandleFunc("/redoc", srv.handleRedoc)
	mux.HandleFunc("/scalar", srv.handleScalar)

	mux.HandleFunc("/api/detect", srv.handleDetect)
	mux.HandleFunc("/api/project", srv.handleProject)
	mux.HandleFunc("/api/default-framework", srv.handleDefaultFramework)
	mux.HandleFunc("/api/generate", srv.handleGenerate)
	mux.HandleFunc("/api/spec.json", srv.handleSpecJSON)
	mux.HandleFunc("/api/spec.yaml", srv.handleSpecYAML)
	mux.HandleFunc("/api/config.yaml", srv.handleConfigYAML)
	mux.HandleFunc("/api/default-config.yaml", srv.handleDefaultConfigYAML)
	mux.HandleFunc("/api/render-config", srv.handleRenderConfig)
	mux.HandleFunc("/api/parse-config", srv.handleParseConfig)
	mux.HandleFunc("/api/browse", srv.handleBrowse)
	mux.HandleFunc("/api/metadata-summary", srv.handleMetadataSummary)
	mux.HandleFunc("/api/health", srv.handleHealth)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	log.Printf("🛠  apispec-ui starting on http://%s", addr)
	log.Printf("📁 Project: %s", cfg.InputDir)
	log.Printf("    Open http://%s in your browser to configure & preview", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func parseFlags() *ServerConfig {
	cfg := &ServerConfig{}
	flag.StringVar(&cfg.Host, "host", "localhost", "HTTP host to bind")
	flag.IntVar(&cfg.Port, "port", 8088, "HTTP port to listen on")
	flag.StringVar(&cfg.InputDir, "dir", ".", "Go project directory to analyze")
	flag.StringVar(&cfg.InputDir, "d", ".", "Shorthand for --dir")
	flag.StringVar(&cfg.ConfigFile, "config", "", "Optional initial APISpec config YAML to seed the UI")
	flag.StringVar(&cfg.ConfigFile, "c", "", "Shorthand for --config")
	flag.BoolVar(&cfg.Verbose, "verbose", false, "Verbose logging")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "apispec-ui: interactive web UI to configure and preview an OpenAPI spec\n\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [flags]\n\nFlags:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s -d ./examples/api -port 8088\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -d . -c apispec.yaml\n", os.Args[0])
	}
	flag.Parse()

	abs, err := filepath.Abs(cfg.InputDir)
	if err == nil {
		cfg.InputDir = abs
	}
	return cfg
}

// --- helpers --------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("failed to encode JSON: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readAsset(name string) ([]byte, error) {
	return assets.ReadFile("assets/" + name)
}

// defaultConfigForFramework returns the default APISpecConfig for the named
// framework, falling back to net/http.
func defaultConfigForFramework(name string) *spec.APISpecConfig {
	switch strings.ToLower(name) {
	case "gin":
		return spec.DefaultGinConfig()
	case "chi":
		return spec.DefaultChiConfig()
	case "echo":
		return spec.DefaultEchoConfig()
	case "fiber":
		return spec.DefaultFiberConfig()
	case "mux":
		return spec.DefaultMuxConfig()
	default:
		return spec.DefaultHTTPConfig()
	}
}

// findModuleRoot walks up from start looking for a go.mod file.
func findModuleRoot(start string) (string, string) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return start, ""
	}
	for {
		gomod := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(gomod); err == nil {
			return dir, readModulePath(gomod)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return start, ""
		}
		dir = parent
	}
}

func readModulePath(gomod string) string {
	f, err := os.Open(gomod)
	if err != nil {
		return ""
	}
	defer func() {
		err = f.Close()
		if err != nil {
			log.Printf("failed to close %s: %v", gomod, err)
		}
	}()
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module"))
		}
	}
	return ""
}

// --- handlers -------------------------------------------------------------

func (s *UIServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	body, err := readAsset("config_ui.html")
	if err != nil {
		http.Error(w, "UI template missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(body)
}

func (s *UIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":         true,
		"hasSpec":    s.currentSpec != nil,
		"lastGenAt":  s.lastGen,
		"lastError":  s.lastErr,
		"projectDir": s.inputDir,
	})
}

// validateProjectDir resolves dir to an absolute path and confirms it exists
// and is a directory. Returns the abs path or an error message.
func validateProjectDir(dir string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", fmt.Errorf("dir is empty")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("cannot resolve path: %w", err)
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("cannot stat path: %w", err)
	}
	if !st.IsDir() {
		return "", fmt.Errorf("not a directory: %s", abs)
	}
	return abs, nil
}

// buildDetectResponse runs framework detection on dir and builds the full
// pre-fill response (info, servers, security, full FrameworkConfig, etc.)
// using either an explicitly-loaded config file or the framework defaults.
func (s *UIServer) buildDetectResponse(dir string) DetectResponse {
	root, modPath := findModuleRoot(dir)

	det := core.NewFrameworkDetector()
	framework, err := det.Detect(root)
	if err != nil || framework == "" {
		framework = "net/http"
	}

	var base *spec.APISpecConfig
	if s.cfg.ConfigFile != "" {
		if loaded, lerr := spec.LoadAPISpecConfig(s.cfg.ConfigFile); lerr == nil {
			base = loaded
		} else if s.cfg.Verbose {
			log.Printf("could not load --config %s: %v", s.cfg.ConfigFile, lerr)
		}
	}
	if base == nil {
		base = defaultConfigForFramework(framework)
	}

	if base.Info.Title == "" {
		title := filepath.Base(modPath)
		if title == "" || title == "." {
			title = filepath.Base(root)
		}
		base.Info.Title = title + " API"
	}
	if base.Info.Version == "" {
		base.Info.Version = "1.0.0"
	}
	if len(base.Servers) == 0 {
		base.Servers = []spec.Server{
			{URL: "http://localhost:8080", Description: "Local development"},
		}
	}

	return DetectResponse{
		InputDir:            dir,
		ModuleRoot:          root,
		ModulePath:          modPath,
		DetectedFramework:   framework,
		SupportedFrameworks: supportedFrameworks,
		OpenAPIVersion:      "3.1.0",
		Info:                base.Info,
		Servers:             base.Servers,
		Security:            base.Security,
		SecuritySchemes:     base.SecuritySchemes,
		Tags:                base.Tags,
		ExternalDocs:        base.ExternalDocs,
		Defaults:            base.Defaults,
		TypeMapping:         base.TypeMapping,
		ExternalTypes:       base.ExternalTypes,
		Overrides:           base.Overrides,
		Include:             base.Include,
		Exclude:             base.Exclude,
		Framework:           base.Framework,
	}
}

func (s *UIServer) handleDetect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	writeJSON(w, http.StatusOK, s.buildDetectResponse(s.currentDir()))
}

// handleProject switches the active project directory (validating it exists)
// and returns a fresh DetectResponse so the UI can rebuild the form from
// the new project's go.mod and detected framework.
func (s *UIServer) handleProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var req ProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	abs, err := validateProjectDir(req.Dir)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.setDir(abs)
	log.Printf("📁 Project switched to: %s", abs)
	writeJSON(w, http.StatusOK, s.buildDetectResponse(abs))
}

// handleDefaultFramework returns just the FrameworkConfig (route/request/
// response/param/mount patterns) for a named framework. Used by the UI to
// reload the pattern editors when the user changes the framework selector.
func (s *UIServer) handleDefaultFramework(w http.ResponseWriter, r *http.Request) {
	fw := r.URL.Query().Get("framework")
	if fw == "" {
		fw = "net/http"
	}
	cfg := defaultConfigForFramework(fw)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"framework":       fw,
		"frameworkConfig": cfg.Framework,
	})
}

func (s *UIServer) handleGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}

	var req GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	if req.Framework == "" {
		req.Framework = "net/http"
	}
	if req.OpenAPIVersion == "" {
		req.OpenAPIVersion = "3.1.0"
	}

	apiCfg, err := buildAPISpecConfig(&req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	start := time.Now()

	engCfg := &engine.EngineConfig{
		InputDir:                     s.currentDir(),
		Title:                        req.Info.Title,
		APIVersion:                   req.Info.Version,
		Description:                  req.Info.Description,
		OpenAPIVersion:               req.OpenAPIVersion,
		APISpecConfig:                apiCfg,
		MaxNodesPerTree:              engine.DefaultMaxNodesPerTree,
		MaxChildrenPerNode:           engine.DefaultMaxChildrenPerNode,
		MaxArgsPerFunction:           engine.DefaultMaxArgsPerFunction,
		MaxNestedArgsDepth:           engine.DefaultMaxNestedArgsDepth,
		MaxRecursionDepth:            engine.DefaultMaxRecursionDepth,
		SkipCGOPackages:              true,
		AnalyzeFrameworkDependencies: true,
		AutoIncludeFrameworkPackages: true,
		AutoExcludeTests:             true,
		AutoExcludeMocks:             true,
		Verbose:                      s.cfg.Verbose,
	}
	if req.Info.Contact != nil {
		engCfg.ContactName = req.Info.Contact.Name
		engCfg.ContactURL = req.Info.Contact.URL
		engCfg.ContactEmail = req.Info.Contact.Email
	}
	if req.Info.License != nil {
		engCfg.LicenseName = req.Info.License.Name
		engCfg.LicenseURL = req.Info.License.URL
	}

	gen := engine.NewEngine(engCfg)
	out, err := gen.GenerateOpenAPI()
	if err != nil {
		s.mu.Lock()
		s.lastErr = err.Error()
		s.mu.Unlock()
		writeError(w, http.StatusInternalServerError, "generation failed: "+err.Error())
		return
	}

	// The engine already analysed the project to build `out`. Reuse the same
	// metadata for the suggestion summary so we don't pay for the analysis
	// twice.
	var summary *MetadataSummary
	if meta := gen.GetMetadata(); meta != nil {
		summary = summarizeMetadata(meta, gen.ModuleRoot())
	}

	now := time.Now()
	s.mu.Lock()
	s.currentSpec = out
	s.currentCfg = apiCfg
	s.lastGen = now
	s.lastErr = ""
	if summary != nil {
		if s.metaCache == nil {
			s.metaCache = make(map[string]*MetadataSummary)
		}
		s.metaCache[s.inputDir] = summary
	}
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, GenerateResponse{
		OK:          true,
		Framework:   req.Framework,
		PathCount:   len(out.Paths),
		GeneratedAt: now,
		DurationMs:  time.Since(start).Milliseconds(),
	})
}

func (s *UIServer) handleSpecJSON(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	cur := s.currentSpec
	s.mu.RUnlock()
	if cur == nil {
		writeError(w, http.StatusNotFound, "no spec generated yet — POST /api/generate first")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(cur); err != nil {
		log.Printf("failed to encode spec JSON: %v", err)
	}
}

func (s *UIServer) handleSpecYAML(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	cur := s.currentSpec
	s.mu.RUnlock()
	if cur == nil {
		writeError(w, http.StatusNotFound, "no spec generated yet")
		return
	}
	w.Header().Set("Content-Type", "application/x-yaml; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	if err := enc.Encode(cur); err != nil {
		log.Printf("failed to encode spec YAML: %v", err)
	}
	_ = enc.Close()
}

func (s *UIServer) handleConfigYAML(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	cur := s.currentCfg
	s.mu.RUnlock()
	if cur == nil {
		writeError(w, http.StatusNotFound, "no config — generate a spec first")
		return
	}
	w.Header().Set("Content-Type", "application/x-yaml; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=apispec.yaml")
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	if err := enc.Encode(cur); err != nil {
		log.Printf("failed to encode config YAML: %v", err)
	}
	_ = enc.Close()
}

// --- browse + metadata helpers --------------------------------------------

// BrowseEntry is a single subdirectory in a browse listing.
type BrowseEntry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	HasGoMod bool   `json:"hasGoMod"`
}

// BrowseResponse is the result of GET /api/browse.
type BrowseResponse struct {
	Path     string        `json:"path"`     // resolved absolute path
	Parent   string        `json:"parent"`   // parent dir, empty at filesystem root
	HasGoMod bool          `json:"hasGoMod"` // true if Path itself has a go.mod
	Entries  []BrowseEntry `json:"entries"`
}

// handleBrowse lists subdirectories of `?path=...` so the UI can offer a
// folder picker. Hidden directories (.git, .vscode, …) are skipped.
func (s *UIServer) handleBrowse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	p := r.URL.Query().Get("path")
	if strings.TrimSpace(p) == "" {
		if home, err := os.UserHomeDir(); err == nil {
			p = home
		} else {
			p = "/"
		}
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad path: "+err.Error())
		return
	}
	st, err := os.Stat(abs)
	if err != nil {
		writeError(w, http.StatusBadRequest, "cannot stat: "+err.Error())
		return
	}
	if !st.IsDir() {
		writeError(w, http.StatusBadRequest, "not a directory: "+abs)
		return
	}

	dirEntries, err := os.ReadDir(abs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read failed: "+err.Error())
		return
	}

	var out []BrowseEntry
	for _, e := range dirEntries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		sub := filepath.Join(abs, name)
		_, gerr := os.Stat(filepath.Join(sub, "go.mod"))
		out = append(out, BrowseEntry{
			Name:     name,
			Path:     sub,
			HasGoMod: gerr == nil,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		// projects (with go.mod) float to the top, then alpha
		if out[i].HasGoMod != out[j].HasGoMod {
			return out[i].HasGoMod
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})

	parent := filepath.Dir(abs)
	if parent == abs {
		parent = ""
	}
	_, gerr := os.Stat(filepath.Join(abs, "go.mod"))

	writeJSON(w, http.StatusOK, BrowseResponse{
		Path:     abs,
		Parent:   parent,
		HasGoMod: gerr == nil,
		Entries:  out,
	})
}

// MetadataSummary surfaces just enough of the analyzed project to drive
// autocomplete/suggestion UI. It is intentionally lightweight — we don't
// ship the full call graph or string pool to the browser.
type MetadataSummary struct {
	ModuleRoot   string   `json:"moduleRoot"`
	ModulePath   string   `json:"modulePath"`
	Packages     []string `json:"packages"`     // analyzed packages (incl. third-party in scope)
	UserPackages []string `json:"userPackages"` // subset prefixed by ModulePath
	Functions    []string `json:"functions"`    // qualified `pkg.Func` (top-level)
	Methods      []string `json:"methods"`      // qualified `pkg.(Receiver).Method`
	Types        []string `json:"types"`        // qualified `pkg.Type`
	Imports      []string `json:"imports"`      // import paths NOT in user module (third-party)
}

// handleMetadataSummary returns the metadata summary that was extracted as a
// side-effect of the most recent successful /api/generate. We deliberately do
// NOT run a separate analysis pass here — running the engine twice (once for
// the OpenAPI spec, once for the suggestions) doubles the wall-clock cost on
// every project change. Generate writes the cache; this endpoint just reads.
func (s *UIServer) handleMetadataSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	dir := s.currentDir()

	s.mu.RLock()
	cached, ok := s.metaCache[dir]
	s.mu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound,
			"no metadata yet — click Generate to analyze the project (suggestions populate after the first run)")
		return
	}
	writeJSON(w, http.StatusOK, cached)
}

// summarizeMetadata flattens *metadata.Metadata into the summary the UI
// needs. Map keys (function names, type names, package paths) are already
// strings; only Method.Name/Receiver need StringPool resolution.
func summarizeMetadata(meta *metadata.Metadata, moduleRoot string) *MetadataSummary {
	out := &MetadataSummary{
		ModuleRoot: moduleRoot,
		ModulePath: meta.CurrentModulePath,
	}
	if out.ModulePath == "" {
		out.ModulePath = readModulePath(filepath.Join(moduleRoot, "go.mod"))
	}

	pkgSet := map[string]struct{}{}
	fnSet := map[string]struct{}{}
	mSet := map[string]struct{}{}
	tSet := map[string]struct{}{}
	importSet := map[string]struct{}{}

	for pkgPath, pkg := range meta.Packages {
		pkgSet[pkgPath] = struct{}{}
		if pkg == nil {
			continue
		}
		for _, file := range pkg.Files {
			if file == nil {
				continue
			}
			for fnName := range file.Functions {
				fnSet[pkgPath+"."+fnName] = struct{}{}
			}
			for typeName, typ := range file.Types {
				tSet[pkgPath+"."+typeName] = struct{}{}
				if typ == nil {
					continue
				}
				for _, m := range typ.Methods {
					name := meta.StringPool.GetString(m.Name)
					recv := meta.StringPool.GetString(m.Receiver)
					if name == "" {
						continue
					}
					if recv == "" {
						recv = typeName
					}
					mSet[fmt.Sprintf("%s.(%s).%s", pkgPath, recv, name)] = struct{}{}
				}
			}
			// also walk package-level Types map (the merged view)
			for _, alias := range file.Imports {
				path := meta.StringPool.GetString(alias)
				if path != "" {
					importSet[path] = struct{}{}
				}
			}
		}
		for typeName, typ := range pkg.Types {
			tSet[pkgPath+"."+typeName] = struct{}{}
			if typ == nil {
				continue
			}
			for _, m := range typ.Methods {
				name := meta.StringPool.GetString(m.Name)
				recv := meta.StringPool.GetString(m.Receiver)
				if name == "" {
					continue
				}
				if recv == "" {
					recv = typeName
				}
				mSet[fmt.Sprintf("%s.(%s).%s", pkgPath, recv, name)] = struct{}{}
			}
		}
	}

	out.Packages = sortedKeys(pkgSet)
	out.Functions = sortedKeys(fnSet)
	out.Methods = sortedKeys(mSet)
	out.Types = sortedKeys(tSet)

	// Imports = paths that are not in our analyzed packages
	for imp := range importSet {
		if _, isLocal := pkgSet[imp]; isLocal {
			continue
		}
		out.Imports = append(out.Imports, imp)
	}
	sort.Strings(out.Imports)

	// User packages = those prefixed by ModulePath
	if out.ModulePath != "" {
		for _, p := range out.Packages {
			if p == out.ModulePath || strings.HasPrefix(p, out.ModulePath+"/") {
				out.UserPackages = append(out.UserPackages, p)
			}
		}
	}
	return out
}

func sortedKeys(m map[string]struct{}) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// buildAPISpecConfig merges a GenerateRequest into a full *APISpecConfig
// using the same rules as handleGenerate. Used by /api/generate (to drive
// the engine) and /api/render-config (to render the YAML preview).
func buildAPISpecConfig(req *GenerateRequest) (*spec.APISpecConfig, error) {
	if req.Framework == "" {
		req.Framework = "net/http"
	}

	if req.UseRawConfig && strings.TrimSpace(req.RawConfig) != "" {
		cfg := &spec.APISpecConfig{}
		if err := yaml.Unmarshal([]byte(req.RawConfig), cfg); err != nil {
			return nil, fmt.Errorf("invalid YAML in rawConfig: %w", err)
		}
		return cfg, nil
	}

	cfg := defaultConfigForFramework(req.Framework)
	cfg.Info = req.Info
	cfg.Servers = req.Servers
	cfg.Security = req.Security
	if req.SecuritySchemes != nil {
		cfg.SecuritySchemes = req.SecuritySchemes
	}
	if len(req.Tags) > 0 {
		cfg.Tags = req.Tags
	}
	if req.ExternalDocs != nil && (req.ExternalDocs.URL != "" || req.ExternalDocs.Description != "") {
		cfg.ExternalDocs = req.ExternalDocs
	}
	if req.Defaults.RequestContentType != "" || req.Defaults.ResponseContentType != "" || req.Defaults.ResponseStatus != 0 {
		cfg.Defaults = req.Defaults
	}
	if len(req.TypeMapping) > 0 {
		cfg.TypeMapping = append(cfg.TypeMapping, req.TypeMapping...)
	}
	if len(req.ExternalTypes) > 0 {
		cfg.ExternalTypes = append(cfg.ExternalTypes, req.ExternalTypes...)
	}
	if len(req.Overrides) > 0 {
		cfg.Overrides = append(cfg.Overrides, req.Overrides...)
	}
	cfg.Include = req.Include
	cfg.Exclude = req.Exclude

	if req.FrameworkConfig != nil {
		fc := req.FrameworkConfig
		if fc.RoutePatterns != nil {
			cfg.Framework.RoutePatterns = fc.RoutePatterns
		}
		if fc.RequestBodyPatterns != nil {
			cfg.Framework.RequestBodyPatterns = fc.RequestBodyPatterns
		}
		if fc.ResponsePatterns != nil {
			cfg.Framework.ResponsePatterns = fc.ResponsePatterns
		}
		if fc.ParamPatterns != nil {
			cfg.Framework.ParamPatterns = fc.ParamPatterns
		}
		if fc.MountPatterns != nil {
			cfg.Framework.MountPatterns = fc.MountPatterns
		}
	}
	return cfg, nil
}

// handleRenderConfig accepts the same body as /api/generate and returns the
// merged APISpecConfig as YAML — without running the engine. The UI uses
// this to keep its Full-YAML editor in sync with the structured form.
func (s *UIServer) handleRenderConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var req GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	cfg, err := buildAPISpecConfig(&req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/x-yaml; charset=utf-8")
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	if err := enc.Encode(cfg); err != nil {
		log.Printf("encode render config YAML: %v", err)
	}
	_ = enc.Close()
}

// handleParseConfig accepts {"yaml": "..."} and returns the parsed
// APISpecConfig as JSON. The UI's "Apply YAML to form" button uses this to
// turn raw YAML edits back into structured form state.
func (s *UIServer) handleParseConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var body struct {
		YAML string `json:"yaml"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	cfg := &spec.APISpecConfig{}
	if err := yaml.Unmarshal([]byte(body.YAML), cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid YAML: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// handleDefaultConfigYAML returns the YAML for the named framework's default
// APISpecConfig — the UI uses this to seed the Full-YAML editor with a
// complete, syntactically-correct starting point.
func (s *UIServer) handleDefaultConfigYAML(w http.ResponseWriter, r *http.Request) {
	fw := r.URL.Query().Get("framework")
	if fw == "" {
		fw = "net/http"
	}
	cfg := defaultConfigForFramework(fw)
	w.Header().Set("Content-Type", "application/x-yaml; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	if err := enc.Encode(cfg); err != nil {
		log.Printf("encode default config YAML: %v", err)
	}
	_ = enc.Close()
}

func (s *UIServer) handleSwagger(w http.ResponseWriter, r *http.Request) {
	body, err := readAsset("swagger.html")
	if err != nil {
		http.Error(w, "swagger UI template missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(body)
}

func (s *UIServer) handleRedoc(w http.ResponseWriter, r *http.Request) {
	body, err := readAsset("redoc.html")
	if err != nil {
		http.Error(w, "redoc template missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(body)
}

func (s *UIServer) handleScalar(w http.ResponseWriter, r *http.Request) {
	body, err := readAsset("scalar.html")
	if err != nil {
		http.Error(w, "scalar template missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(body)
}
