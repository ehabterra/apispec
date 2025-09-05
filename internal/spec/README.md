# Spec Package - OpenAPI Specification Generation

This package provides a clean, organized system for generating OpenAPI specifications from Go code metadata. It's designed to be framework-agnostic and easily extensible.

## Architecture

The spec package is organized into several focused components:

### 1. Configuration (`config.go`)

The configuration system is framework-agnostic and uses pattern-based extraction:

```go
type APISpecConfig struct {
    Framework FrameworkConfig `yaml:"framework"`  // Framework-specific patterns
    TypeMapping []TypeMapping `yaml:"typeMapping"` // Go to OpenAPI type mappings
    Overrides []Override `yaml:"overrides"`        // Manual overrides
    Include IncludeExclude `yaml:"include"`        // Include/exclude filters
    Exclude IncludeExclude `yaml:"exclude"`
    Defaults Defaults `yaml:"defaults"`            // Default values
    // OpenAPI metadata
    Info Info `yaml:"info"`
    Servers []Server `yaml:"servers"`
    // ... other OpenAPI fields
}
```

#### Framework Configuration

Each framework has its own extraction patterns:

```go
type FrameworkConfig struct {
    RoutePatterns       []RoutePattern       `yaml:"routePatterns"`
    RequestBodyPatterns []RequestBodyPattern `yaml:"requestBodyPatterns"`
    ResponsePatterns    []ResponsePattern    `yaml:"responsePatterns"`
    ParamPatterns       []ParamPattern       `yaml:"paramPatterns"`
    MountPatterns       []MountPattern       `yaml:"mountPatterns"`
}
```

#### Pattern-Based Extraction

Patterns define how to extract information from function calls:

```go
type RoutePattern struct {
    CallRegex         string   `yaml:"callRegex,omitempty"`         // e.g., '^BindJSON$'
    FunctionNameRegex string   `yaml:"functionNameRegex,omitempty"` // e.g., '.*Handler$'
    MethodFromCall    bool     `yaml:"methodFromCall,omitempty"`    // Extract method from function name
    PathFromArg       bool     `yaml:"pathFromArg,omitempty"`       // Extract path from argument
    HandlerFromArg    bool     `yaml:"handlerFromArg,omitempty"`    // Extract handler from argument
    PathArgIndex      int      `yaml:"pathArgIndex,omitempty"`      // Which arg contains path
    HandlerArgIndex   int      `yaml:"handlerArgIndex,omitempty"`   // Which arg contains handler
}
```

### 2. Extraction (`extractors.go`)

The extractor system uses the configuration patterns to extract information from metadata:

```go
type Extractor struct {
    meta *metadata.Metadata
    cfg  *APISpecConfig
}

func (e *Extractor) ExtractRoutes() []RouteInfo
```

#### Route Information

```go
type RouteInfo struct {
    Path     string
    Method   string
    Handler  string
    Package  string
    File     string
    Function string
    Summary  string
    Tags     []string
    Request  *RequestInfo
    Response *ResponseInfo
    Params   []Parameter
}
```

### 3. Mapping (`mapper.go`)

The mapper converts extracted route information into OpenAPI specifications:

```go
func MapMetadataToOpenAPI(meta *metadata.Metadata, cfg *APISpecConfig, genCfg GeneratorConfig) (*OpenAPISpec, error)
```

### 4. OpenAPI Types (`openapi.go`)

Complete OpenAPI 3.0 specification types for building the final output.

## Usage

### Basic Usage

```go
// Load configuration
cfg, err := LoadAPISpecConfig("apispec.yaml")
if err != nil {
    log.Fatal(err)
}

// Use framework-specific defaults
cfg = DefaultChiConfig()

// Generate OpenAPI spec
genCfg := GeneratorConfig{
    OpenAPIVersion: "3.0.3",
    Title:          "My API",
    APIVersion:     "1.0.0",
}

spec, err := MapMetadataToOpenAPI(metadata, cfg, genCfg)
if err != nil {
    log.Fatal(err)
}
```

### Configuration Examples

#### Chi Router Configuration

```yaml
framework:
  routePatterns:
    - callRegex: "(?i)(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD)$"
      methodFromCall: true
      pathFromArg: true
      handlerFromArg: true
      pathArgIndex: 0
      handlerArgIndex: 1
  
  requestBodyPatterns:
    - callRegex: "(?i)(POST|PUT|PATCH)$"
      bodyArgIndex: 1
      typeFromArg: true
  
  responsePatterns:
    - callRegex: "(?i)(JSON|String|XML|YAML|ProtoBuf|Data|File|Redirect)$"
      statusArgIndex: 0
      responseArgIndex: 1
      statusFromArg: true

defaults:
  requestContentType: "application/json"
  responseContentType: "application/json"
  responseStatus: 200

info:
  title: "My API"
  version: "1.0.0"
```

#### Gin Framework Configuration

```yaml
framework:
  routePatterns:
    - callRegex: "(?i)(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD)$"
      methodFromCall: true
      pathFromArg: true
      handlerFromArg: true
      pathArgIndex: 0
      handlerArgIndex: 1
  
  requestBodyPatterns:
    - callRegex: "(?i)(BindJSON|BindXML|BindYAML|BindForm|BindQuery)$"
      bodyArgIndex: 0
      typeFromArg: true
  
  responsePatterns:
    - callRegex: "(?i)(JSON|String|XML|YAML|ProtoBuf|Data|File|Redirect)$"
      statusArgIndex: 0
      responseArgIndex: 1
      statusFromArg: true
```

### Type Mappings

Map Go types to OpenAPI schemas:

```yaml
typeMapping:
  - goType: "time.Time"
    openapiType:
      type: "string"
      format: "date-time"
  
  - goType: "uuid.UUID"
    openapiType:
      type: "string"
      format: "uuid"
```

### Overrides

Provide manual overrides for specific functions:

```yaml
overrides:
  - functionName: "CreateUser"
    summary: "Create a new user"
    description: "Creates a new user with the provided information"
    responseStatus: 201
    tags: ["users"]
```

## Best Practices

### 1. Framework-Agnostic Design

- Use pattern-based extraction instead of framework-specific code
- Define clear interfaces for different extraction types
- Make configuration the primary driver of behavior

### 2. Clean Separation of Concerns

- **Configuration**: Defines what to extract and how
- **Extraction**: Uses patterns to extract information from metadata
- **Mapping**: Converts extracted information to OpenAPI format
- **Types**: Provides complete OpenAPI specification structures

### 3. Extensibility

- Add new patterns by extending the pattern structs
- Support new frameworks by creating new default configurations
- Extend type mappings for custom Go types

### 4. Error Handling

- Graceful degradation when patterns don't match
- Clear error messages for configuration issues
- Fallback to defaults when extraction fails

### 5. Performance

- Efficient string pool usage for metadata
- Minimal memory allocations during extraction
- Lazy evaluation of complex patterns

## Migration from Old System

The new system replaces the complex `mapper.go` with:

1. **Clean extractors** that focus on pattern matching
2. **Framework-agnostic configuration** that's easy to understand
3. **Organized OpenAPI types** for complete specification support
4. **Simple mapping logic** that builds specs from extracted data

### Key Improvements

- **Framework Independence**: No hardcoded framework logic
- **Pattern-Based**: Uses regex and call chain patterns
- **Configuration-Driven**: Easy to customize for different frameworks
- **Clean Architecture**: Clear separation between extraction and mapping
- **Extensible**: Easy to add new patterns and frameworks

## Future Enhancements

1. **Plugin System**: Allow custom extractors and mappers
2. **Validation**: Validate configuration and extracted data
3. **Caching**: Cache extracted information for performance
4. **Testing**: Comprehensive test suite for all patterns
5. **Documentation**: Auto-generated documentation from patterns 