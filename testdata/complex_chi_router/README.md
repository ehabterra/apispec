# Complex Chi Router Testdata Example

This testdata example demonstrates a complex Chi router structure that follows the pattern shared by a developer:

## Structure

```
complex_chi_router/
├── main.go              # Main router with middleware and mounting
├── handler/
│   └── handler.go       # Main handler with Routes() method
├── auth/
│   └── handler.go       # Authentication routes
├── user/
│   └── handler.go       # User CRUD operations
├── models/
│   ├── user.go          # User-related models
│   └── auth.go          # Authentication models
└── go.mod               # Dependencies
```

## Router Pattern

The example follows this exact pattern:

### main.go
```go
r := chi.NewRouter()
r.Use(middlewares...)
r.Mount("/api", handler.New(...).Routes())
```

### handler/handler.go
```go
func (h *Handler) Routes() http.Handler {
    r := chi.NewRouter()
    r.Mount("/auth", auth.Routes())
    
    r.Group(func(rg chi.Router) {
        rg.Use(h.authMiddleware)
        rg.Mount("/user", user.Routes())
    })
    return r
}
```

### user/user.go
```go
func (h *Handler) Routes() http.Handler {
    r := chi.NewRouter()
    r.Get("/", h.list)
    r.Get("/{name}", h.show)
    r.Post("/create", h.create)
    return r
}
```

## API Endpoints

The resulting API should have these endpoints:

- `GET /api/auth/login` - User login
- `POST /api/auth/register` - User registration
- `POST /api/auth/refresh` - Token refresh
- `POST /api/auth/logout` - User logout (protected)
- `GET /api/auth/me` - Get current user (protected)
- `GET /api/user/` - List users (protected)
- `GET /api/user/{name}` - Get user by name (protected)
- `POST /api/user/create` - Create user (protected)
- `PUT /api/user/{id}` - Update user (protected)
- `DELETE /api/user/{id}` - Delete user (protected)
- `GET /api/user/{id}/profile` - Get user profile (protected)
- `PUT /api/user/{id}/profile` - Update user profile (protected)
- `GET /api/user/search` - Search users (protected)

## Testing APISpec

To test APISpec with this example:

```bash
# Basic generation
./apispec --dir testdata/complex_chi_router --output openapi.yaml

# With framework analysis
./apispec --dir testdata/complex_chi_router --output openapi.yaml --analyze-framework-dependencies

# With call graph diagram
./apispec --dir testdata/complex_chi_router --output openapi.yaml --diagram

# With metadata for debugging
./apispec --dir testdata/complex_chi_router --output openapi.yaml --write-metadata
```

## Key Features Demonstrated

1. **Mounted Routes**: Routes mounted from separate packages using `Routes()` methods
2. **Nested Groups**: Route groups with middleware applied
3. **Complex Call Graph**: Multiple levels of route mounting and handler resolution
4. **Parameter Extraction**: Path parameters like `{id}`, `{name}`
5. **Request/Response Types**: Proper type inference from struct tags and validation
6. **Middleware Integration**: Authentication and other middleware patterns
7. **Render Package Support**: Full support for `github.com/go-chi/render` functions:
   - `render.DecodeJSON()` - Request body decoding
   - `render.JSON()` - JSON response rendering
   - `render.Status()` - HTTP status code setting

This example helps test APISpec's ability to handle real-world complex router structures that many Go developers use in production applications.
