# Go Style Guidelines

This document provides Go-specific coding standards for the dendrite-pulse project.
These guidelines are based on the
[Google Go Style Best Practices](https://google.github.io/styleguide/go/best-practices.html).

## Shared Types

### Avoid Duplicate Type Definitions

Do not define the same struct types in multiple packages. When types are needed across packages,
place them in a shared location.

**JSON:API response types** belong in `internal/api/`. This includes:

- Response envelopes
- Pagination structures (`PaginationMeta`, `PaginationLinks`)
- Error response types

> [!IMPORTANT]
> Before creating a new struct type, check if a similar type already exists in `internal/api/`
> or another shared package.

Bad — duplicating types across packages:

```go
// internal/ping/types.go
type Response struct {
    Meta PaginationMeta `json:"meta"`
    Data Resource       `json:"data"`
}

// internal/files/handler.go
type Response struct {
    Meta PaginationMeta `json:"meta"`
    Data []Resource     `json:"data"`
}
```

Good — shared types in a common package:

```go
// internal/api/response.go
type CollectionResponse struct {
    Meta  *PaginationMeta `json:"meta,omitempty"`
    Data  []Resource      `json:"data"`
    Links *PaginationLinks `json:"links,omitempty"`
}

type SingleResponse struct {
    Data Resource `json:"data"`
}
```

## Error Handling

### Use Sentinel Errors for Programmatic Handling

Define sentinel errors when callers need to distinguish between error types programmatically.
Use `errors.Is()` for comparison.

**When to use sentinel errors:**

- The caller needs to take different actions based on error type
- The error represents a well-defined domain condition
- Multiple call sites may return the same error

**When inline errors are acceptable:**

- The error message alone is sufficient for debugging
- Callers will not branch on the specific error type
- The error is unique to one location

```go
// Define sentinel errors at package level
var (
    ErrRootNotFound = errors.New("file root not found")
    ErrOutsideRoot  = errors.New("path escapes configured root")
)

// Use sentinel errors in functions
func (s *Service) Describe(ctx context.Context, virtual, rel string) (Descriptor, error) {
    root, ok := s.lookupRoot(virtual)
    if !ok {
        return Descriptor{}, fmt.Errorf("%w: %s", ErrRootNotFound, virtual)
    }
    // ...
}

// Caller can check error type
desc, err := svc.Describe(ctx, virtual, rel)
if errors.Is(err, files.ErrRootNotFound) {
    // Handle missing root specifically
}
```

### Error Wrapping

Always place `%w` at the end of format strings when wrapping errors:

```go
return fmt.Errorf("load config: %w", err)
```

## Concurrency Documentation

### Document Thread Safety

Document whether types are safe for concurrent use. Go users assume read-only operations are
safe for concurrent access, but this should be made explicit.

Add a comment to types that are designed for concurrent use:

```go
// Service exposes file operations scoped to configured roots.
// Service is safe for concurrent use after construction.
type Service struct {
    roots   map[string]Root
    ordered []Root
}
```

If a type is NOT safe for concurrent use, document this clearly:

```go
// Builder accumulates configuration. It is NOT safe for concurrent use.
// Call Build() to obtain a thread-safe instance.
type Builder struct {
    // ...
}
```

## Function Documentation

### Document Security-Critical Functions

Unexported functions that implement security-sensitive logic must have documentation explaining
their purpose and constraints.

Priority functions to document:

- Path validation and sanitization
- Input parsing that affects security
- Permission checks

```go
// cleanRelativePath validates and normalizes a relative path,
// preventing directory traversal attacks via ".." segments.
// Returns ErrOutsideRoot if the path would escape the root directory.
func cleanRelativePath(rel string) (string, error) {
    if hasTraversal(rel) {
        return "", fmt.Errorf("%w: %s", ErrOutsideRoot, rel)
    }
    // ...
}

// matchRoot finds the best matching root for a request path.
// Roots are matched longest-prefix-first to handle nested virtual paths.
func matchRoot(requestPath string, roots []Root) (Root, string, bool) {
    // ...
}
```

## Naming Conventions

### Handler Functions

When a package contains a single handler, a generic name like `handler` is acceptable since the
package name provides context (`ping.handler`).

When a package contains multiple handlers, use descriptive names:

```go
// Single handler in package — acceptable
func handler(c echo.Context) error { ... }

// Multiple handlers — use descriptive names
func listHandler(c echo.Context) error { ... }
func getHandler(c echo.Context) error { ... }
func createHandler(c echo.Context) error { ... }
```

### Avoid Repeating Package Names

Do not repeat the package name in function or type names:

```go
// Bad
package files
func FilesListDirectory() { ... }

// Good
package files
func ListDirectory() { ... }
```

## Channel Directions

Specify a channel direction in function parameters when possible:

```go
// Bad — bidirectional channel when only receiving
func waitForServer(errCh chan error) { ... }

// Good — explicit receive-only channel
func waitForServer(errCh <-chan error) { ... }

```

## Quick Reference

| Guideline          | Action                              |
| ------------------ | ----------------------------------- |
| Sentinel errors    | Define for programmatic handling    |
| Error wrapping     | Use `%w` at end of format string    |
| Thread safety      | Document on type                    |
| Security functions | Add explanatory comments            |
| Handler naming     | Be descriptive when multiple exist  |
