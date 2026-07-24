# Repository Guidelines

## Architecture & Scope

Kitsune Proxy is a cross-platform Go tray application. Its local HTTPS listener selects a configured upstream using `model_id`, forwards the request, and streams the response to the Agent.

This is a transparent proxy: preserve method, path, body, streaming, status, and compatible headers; never translate schemas. Keep routing, transport, TLS, configuration, and tray lifecycle as deep modules behind small interfaces. Provider-specific logic only selects URLs and credentials.

## Project Structure & Module Organization

Use this target layout:

- `cmd/kitsune-proxy/` — executable entry point.
- `internal/app/` — lifecycle and coordination.
- `internal/proxy/` — HTTPS serving and forwarding.
- `internal/router/` — `model_id` resolution.
- `internal/config/` — configuration and validation.
- `internal/tray/` — tray UI and OS integration.
- `assets/` — icons and packaged resources.
- `tests/integration/` — end-to-end tests; unit tests stay beside source.

Use Go file suffixes such as `_windows.go`, `_linux.go`, and `_darwin.go` for platform implementations.

## Build, Test, and Development Commands

- `go mod tidy` — synchronize module dependencies.
- `go run ./cmd/kitsune-proxy` — run the tray application locally.
- `go build ./cmd/kitsune-proxy` — build for the current platform.
- `go test ./...` — run all unit and integration tests.
- `go test -race ./...` — detect concurrency issues where supported.
- `go fmt ./...` and `go vet ./...` — format and statically check the code.

CI should build and test Windows, Linux, and macOS targets.

## Coding Style & Naming Conventions

Keep packages lowercase, singular, and focused. Use Go `MixedCaps` and preserve initialisms such as `ID`, `HTTP`, and `URL`. Document exported identifiers, wrap errors with `%w`, pass `context.Context` first, and inject dependencies through constructors.

## Testing Guidelines

Name tests `*_test.go` and prefer table-driven cases. Use `httptest` and fake upstreams; never contact real providers. Verify routing, passthrough, streaming, cancellation, timeouts, malformed input, upstream failures, and shutdown. Add platform-specific tray and TLS checks.

## Commit & Pull Request Guidelines

With no existing history, use Conventional Commits, for example `feat(router): map model IDs to upstreams`. Pull requests must describe behavior, list verification commands and tested systems, link issues, and include screenshots for tray changes.

## Security & Configuration

Bind locally by default. Never commit tokens, private keys, or user configuration. Provide sanitized examples, restrict certificate and config permissions, and redact credentials and sensitive bodies from logs.

## Agent skills

### Issue tracker

Issues and PRDs are tracked in this repository's GitHub Issues. See `docs/agents/issue-tracker.md`.

### Triage labels

Use the five default canonical triage labels. See `docs/agents/triage-labels.md`.

### Domain docs

Use the single-context domain documentation layout. See `docs/agents/domain.md`.
