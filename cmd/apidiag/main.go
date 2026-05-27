// Copyright 2025 Ehab Terra
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

package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/ehabterra/apispec/internal/diagserver"
)

// Version info - can be injected at build time via -ldflags or detected at runtime.
var (
	Version   = "0.0.1"
	Commit    = "unknown"
	BuildDate = "unknown"
	GoVersion = "unknown"
)

// cliConfig is the flag-parsed form. We translate it into a diagserver.Config.
type cliConfig struct {
	ShowVersion bool

	srv diagserver.Config
}

func detectVersionInfo() {
	if Version != "0.0.1" {
		return
	}

	if info, ok := debug.ReadBuildInfo(); ok {
		if info.GoVersion != "" {
			GoVersion = info.GoVersion
		}
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			Version = info.Main.Version
		}

		hasVCSInfo := false
		isModified := false
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				hasVCSInfo = true
				if len(setting.Value) >= 7 {
					Commit = setting.Value[:7]
				} else {
					Commit = setting.Value
				}
			case "vcs.time":
				hasVCSInfo = true
				BuildDate = setting.Value
			case "vcs.modified":
				if setting.Value == "true" {
					isModified = true
				}
			}
		}

		if isModified && !strings.Contains(Version, "+dirty") {
			Version += "+dirty"
		}
		if hasVCSInfo && Version == "0.0.1" {
			Version = "dev"
		}
	}
}

func printVersion() {
	detectVersionInfo()
	fmt.Printf("apidiag version: %s\n", Version)
	fmt.Printf("Commit: %s\n", Commit)
	fmt.Printf("Build date: %s\n", BuildDate)
	fmt.Printf("Go version: %s\n", GoVersion)
}

func main() {
	cfg := parseFlags()

	if cfg.ShowVersion {
		printVersion()
		os.Exit(0)
	}

	server := diagserver.New(&cfg.srv)
	if err := server.LoadMetadata(); err != nil {
		log.Fatalf("Failed to load metadata: %v", err)
	}

	mux := http.NewServeMux()
	server.RegisterRoutes(mux, diagserver.RouteOptions{UIPath: "/"})

	addr := fmt.Sprintf("%s:%d", cfg.srv.Host, cfg.srv.Port)
	log.Printf("🚀 API Diagram server starting on http://%s", addr)
	if cfg.srv.Verbose {
		log.Printf("📊 Serving %s diagrams for: %s", cfg.srv.DiagramType, cfg.srv.InputDir)
		log.Printf("⚙️  Page size: %d, Max depth: %d", cfg.srv.PageSize, cfg.srv.MaxDepth)
	}

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

func parseFlags() *cliConfig {
	cfg := &cliConfig{}

	flag.BoolVar(&cfg.ShowVersion, "version", false, "Show version information")
	flag.BoolVar(&cfg.ShowVersion, "V", false, "Shorthand for --version")

	flag.IntVar(&cfg.srv.Port, "port", 8080, "Server port")
	flag.StringVar(&cfg.srv.Host, "host", "localhost", "Server host")
	flag.StringVar(&cfg.srv.InputDir, "dir", ".", "Input directory containing Go source files")
	flag.IntVar(&cfg.srv.PageSize, "page-size", 100, "Default page size for pagination")
	flag.IntVar(&cfg.srv.MaxDepth, "max-depth", 3, "Maximum call graph depth")
	flag.BoolVar(&cfg.srv.EnableCORS, "cors", true, "Enable CORS headers")
	flag.DurationVar(&cfg.srv.CacheTimeout, "cache-timeout", 5*time.Minute, "Cache timeout for metadata")
	flag.BoolVar(&cfg.srv.Verbose, "verbose", false, "Enable verbose logging")
	flag.BoolVar(&cfg.srv.Verbose, "v", false, "Shorthand for --verbose")

	flag.BoolVar(&cfg.srv.AnalyzeFrameworkDependencies, "analyze-framework-dependencies", false, "Analyze framework dependencies")
	flag.BoolVar(&cfg.srv.AnalyzeFrameworkDependencies, "afd", false, "Shorthand for --analyze-framework-dependencies")

	flag.BoolVar(&cfg.srv.AutoIncludeFrameworkPackages, "auto-include-framework-packages", false, "Auto-include framework packages")
	flag.BoolVar(&cfg.srv.AutoIncludeFrameworkPackages, "aifp", false, "Shorthand for --auto-include-framework-packages")

	flag.BoolVar(&cfg.srv.AutoExcludeTests, "auto-exclude-tests", false, "Auto-exclude test files")
	flag.BoolVar(&cfg.srv.AutoExcludeTests, "aet", false, "Shorthand for --auto-exclude-tests")

	flag.BoolVar(&cfg.srv.AutoExcludeMocks, "auto-exclude-mocks", false, "Auto-exclude mock files")
	flag.BoolVar(&cfg.srv.AutoExcludeMocks, "aem", false, "Shorthand for --auto-exclude-mocks")

	flag.StringVar(&cfg.srv.DiagramType, "diagram-type", "call-graph", "Diagram type: 'call-graph' or 'tracker-tree'")
	flag.StringVar(&cfg.srv.DiagramType, "dt", "call-graph", "Shorthand for --diagram-type")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "APISpec API Diagram Server - Serves paginated call graph diagrams\n\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [flags]\n\nFlags:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s --dir ./myproject --port 8080\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s --dir ./myproject --page-size 50 --max-depth 2\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s --dir ./myproject --diagram-type tracker-tree\n", os.Args[0])
	}

	flag.Parse()

	if cfg.srv.PageSize < 10 {
		cfg.srv.PageSize = 10
	} else if cfg.srv.PageSize > 1000 {
		cfg.srv.PageSize = 1000
	}

	if cfg.srv.MaxDepth < 1 {
		cfg.srv.MaxDepth = 1
	} else if cfg.srv.MaxDepth > 10 {
		cfg.srv.MaxDepth = 10
	}

	if cfg.srv.DiagramType != "call-graph" && cfg.srv.DiagramType != "tracker-tree" {
		cfg.srv.DiagramType = "call-graph"
	}

	return cfg
}
