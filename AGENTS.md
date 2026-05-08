# AGENTS.md

Instructions for AI coding agents working on this repository.

## Commands

| Task | Command |
|------|---------|
| Build | `make build` |
| Test (all) | `make test` or `go test ./... -v -timeout 60s` |
| Test (single pkg) | `go test ./internal/tools/ -v -timeout 60s` |
| Test (single test) | `go test ./internal/tools/ -run TestClick_Success -v` |
| Lint | `make lint` |
| Run (HTTP) | `make run` |
| Docker (start) | `make docker/start` |
| Docker (logs) | `make docker/logs` |
| Docker (stop) | `make docker/stop` |
| Docker (build) | `make docker/build` |

Always run `make lint && make test` before considering work done. Lint installs `golangci-lint` v2.11.4 to `bin/` automatically if missing.

CI (`.github/workflows/ci.yml`) runs `go vet`, golangci-lint, `go build`, and `go test -short` on Windows, macOS, and Linux. The Makefile test target does **not** pass `-short`, so it runs the full suite locally.

Go 1.24+ required (go.mod specifies 1.26.1).

## Branch Policy

- Never commit directly to `main`. Create a branch first.
- Branch naming: `feat/<desc>`, `fix/<desc>`, `refactor/<desc>`.
- Open a PR against `main` after pushing.

## Architecture

Iris is a stateless MCP server exposing 8 browser-control tools via JSON-RPC 2.0 (HTTP on `POST /mcp`), using chromedp (real Chrome running under Xvfb) for coordinate-based interaction. Internally, the MCP dispatch uses `github.com/sevigo/goframe/agent`.

Module: `github.com/LanthornHQ/iris`

```
cmd/iris/main.go          — entry point: signal handling, .env load, logger init, browser driver, wire tools, serve
internal/mcp/             — MCP server: JSON-RPC 2.0 dispatch, RunHTTP
internal/browser/          — browser.Driver interface + chromedp implementation
internal/tools/            — 8 tools + registry + arg helpers
internal/metrics/           — Prometheus metrics (iris_* prefix)
```

Tools are registered via `ToolRegistry.RegisterAll(server)` in `internal/tools/registry.go` — **not** individually in `main.go`.

The 8 tools: `navigate`, `screenshot`, `click`, `type_text`, `scroll`, `wait_for_stable`, `sleep`, `get_datetime`.

### browser.Driver

`browser.Driver` is an interface with 9 methods: `Navigate`, `Click`, `DoubleClick`, `Type`, `Scroll`, `Screenshot`, `WaitForStable`, `Title`, `Close`. Implementation: `internal/browser/chromedp_driver.go`.

### Tool Argument Handling

JSON-RPC numbers decode as `float64` in Go. Two helpers in `internal/tools/args.go` handle this:
- `intArg(args, name)` — required integer, errors if missing/invalid; clamps to `[0, MaxInt32]`
- `optIntArg(args, name, default)` — optional integer, returns default if absent; clamps to `[0, MaxInt32]`

## Code Conventions

### Go Style

- `context.Context` is always the first parameter. If unused: `_ context.Context`.
- **Never use named return values.** Use plain return types; assign to local variables.
- Errors: always check, never discard. Wrap with `fmt.Errorf("doing X: %w", err)`. Use `errors.New` for static strings (perfsprint enforces this).
- Logging: `log/slog` only. Store `*slog.Logger` as struct field, pass via constructor. No `log` or `fmt.Println`.
- **stdout is reserved for MCP JSON-RPC transport** — never write to stdout from tool code. `slog.SetDefault(logger)` is called in `main()` before any tool is constructed.
- `.env` is loaded automatically at startup via `godotenv.Load()`. Use `os.Getenv()` for env vars; do not manually read `.env`.
- Environment variable prefix: `IRIS_*`.

### Testing

- `github.com/stretchr/testify` — `require` for fatal preconditions, `assert` for non-fatal checks.
- **Do not use `require` inside HTTP handler closures** (testifylint forbids it — `t.Fatal` is unsafe in goroutines). Use `assert` there instead.
- Table-driven tests for parameterized cases.
- Use `t.Setenv()` for env var tests, `httptest.NewServer` for HTTP tests.
- Silenced logger in tests: see `internal/tools/tools_test.go` (`testLogger` variable).
- Mock `browser.Driver` for tool unit tests — see `internal/tools/tools_test.go` and `internal/browser/browser_test.go`.

## Linter Notes

The `.golangci.yml` is strict. Common traps:

- `perfsprint`: `fmt.Errorf("static string")` → `errors.New("static string")`. Only use `fmt.Errorf` with `%w`/`%v`/`%d` etc.
- `nonamedreturns`: No named return values anywhere.
- `testifylint`: Error assertions (`NoError`, `Error`, `ErrorIs`) must use `require` except in HTTP handlers.
- `depguard`: `log` package banned outside `main.go`; `math/rand` banned outside test files (use `math/rand/v2`).
- `nolintlint`: requires specific linter name and explanation comment on every `//nolint` directive.
- `reassign`: all global variable reassignment checked (pattern `".*"`).
- `sloglint`: `no-global: all` — no package-level `slog` calls; always pass `*slog.Logger` as a struct field. `context: scope` — prefer `slog.WithContext` when a context is in scope.