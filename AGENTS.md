# LLM-Proxy Agent Guidelines

## Project Structure

`main.go` is the entrypoint for the `llm-proxy` binary.

**Directory Structure:**
- `cmd/` - CLI wiring and Cobra commands (includes `cli_test.go` with golden-file tests in `cmd/testdata/`)
- `internal/server/` - Server startup logic
- `internal/routes/` - HTTP routing and handler registration
- `internal/handler/` - Upstream-specific logic (Ollama, K8s health)
- `internal/middleware/` - HTTP middleware (request logging)
- `internal/config/` - Viper-backed configuration loading
- `weave/` - Weave tracing client and shared types

**Code Organization:**
- Keep HTTP handlers in `handler.go` (exported `http.HandlerFunc` implementations)
- Extract helpers to `utils.go` in the same package
- Tests live next to code: `*_test.go` files in the same directory

## Build, Test, and Development Commands

Always use `Makefile` targets instead of ad hoc commands:

**Building:**
- `make build` - Build `bin/llm-proxy` from `main.go`
- `make run` - Build and start the local proxy server
- `make clean` - Remove `bin/` and `coverage.out`

**Linting:**
- `make lint` - Run `go fmt ./...` and `go vet ./...`

**Testing:**
- `make test` - Run full test suite with coverage via gotestsum
- `go test ./...` - Run tests without coverage
- `go test ./path/to/package -run TestName` - Run a single test
- `go test ./path/to/package -v` - Run with verbose output
- `go test ./internal/handler/ollama -run TestCopyRequestHeaders` - Example: specific test

**Coverage:**
- `make coverage` - Print per-function coverage
- `make coverage-html` - Open HTML coverage report

**Golden Files:**
- `make update-golden` - Refresh CLI golden files after intentional help-text changes

**Local Development:**
Set required environment variables before running:
```bash
export WANDB_API_KEY="your-key"
export WANDB_PROJECT="your-project"
export OLLAMA_URL="http://localhost:11434"
```

## Coding Style & Naming Conventions

**Go Style:**
- Follow standard Go formatting with `go fmt`
- Use tabs (not spaces) as produced by `go fmt`
- Do not hand-align spacing
- Go version: 1.25.5

**Package Names:**
- Keep short and lowercase: `server`, `routes`, `ollama`, `middleware`
- Avoid stutter: prefer `ollama.Handler` not `ollama.OllamaHandler`

**Naming:**
- Exported: `CamelCase` (e.g., `NewHandler`, `Generate`)
- Unexported: `camelCase` (e.g., `hashString`, `forwardRequest`)
- Tests: `TestXxx` pattern (e.g., `TestGenerate_Streaming`)
- Files: `snake_case.go` for implementation, `snake_case_test.go` for tests

**Error Handling:**
- Check errors immediately, return early on failure
- Use `slog.Error()` with structured key-value pairs: `slog.Error("message", "key", value)`
- Return appropriate HTTP status codes to clients

**Comments:**
- Add godoc comments for exported functions and types
- Keep comments concise and factual
- Example: `// Generate handles POST /api/generate requests`

## Testing Guidelines

**Test Location:**
- Tests live next to code: `*_test.go` in same directory
- Use standard `testing` package (no external frameworks)

**Test Patterns:**
- Use table-driven tests with `t.Run("subtest name", ...)` for subtests
- Mock external dependencies using `httptest` for HTTP handlers
- Use `t.Helper()` in test helper functions
- Clean up resources with `defer`

**Golden File Tests:**
- Located in `cmd/testdata/`
- Run `make update-golden` only when output changes are intentional

## Imports & Dependencies

**Import Organization:**
```go
import (
    // Standard library (alphabetical)
    "bufio"
    "bytes"
    "context"

    // Third-party packages (alphabetical)
    "github.com/google/uuid"
    ollamaapi "github.com/ollama/ollama/api"

    // Internal packages (alphabetical)
    "github.com/dacbd/llm-proxy/weave"
)
```

**Dependency Management:**
- Use `go mod tidy` to clean up imports
- Pin to specific versions in `go.mod`
- Key dependencies: Cobra (CLI), Viper (config), Ollama API client

## Security & Configuration

**Secrets:**
- Never commit real `WANDB_API_KEY` values or private upstream URLs
- Use environment variables over hardcoded config
- Sanitize prompts and model output before logging

**Configuration:**
- Use Viper for config loading (see `internal/config/`)
- Support environment variable overrides
- Validate config at startup

## Commit & Pull Request Guidelines

**Commits:**
- Use short, imperative subjects: `add ollama proxy handler`, `fix header forwarding`
- Keep commits focused and easy to scan
- Reference issues when applicable

**Pull Requests:**
- Include brief summary of changes
- Testing notes: `make test`, `make lint` results
- Include request/response samples for proxy behavior changes
- Ensure all tests pass before requesting review

## Common Tasks

**Adding a New Ollama Endpoint:**
1. Add handler method to `internal/handler/ollama/handler.go`
2. Extract helpers to `internal/handler/ollama/utils.go` if needed
3. Register route in `internal/routes/routes.go`
4. Add tests in `internal/handler/ollama/handler_test.go`
5. Run `make lint && make test`

**Proxying Headers:**
- Use `CopyRequestHeaders(upstream, r)` for standard headers
- Forwards: Authorization, Content-Type, User-Agent, Accept, X-Request-Id, X-Correlation-Id
- Also forwards OpenAI headers (x-stainless-* and OpenAI-*)

**Adding Tracing:**
- Use Weave client methods: `CallStart`, `CallEnd`, `CallRead`
- Fire tracing in background goroutines to avoid blocking requests
- Use `hashString()` for generating span IDs from message content
