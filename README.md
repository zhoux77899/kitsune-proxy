# Kitsune Proxy

Kitsune Proxy is a cross-platform Go system tray application for individuals who use multiple model-provider endpoints. It starts a local HTTPS service, receives requests from AI Agents, selects the configured upstream by `model_id`, and returns the upstream response unchanged.

> [!IMPORTANT]
> The project is currently being initialized. It does not yet provide a runnable release, a stable configuration format, or installation packages.

## How it works

**Request path:** AI Agent → local HTTPS service → route by `model_id` → configured model-provider endpoint. The response returns transparently along the same path.

An Agent connects to one local address instead of switching provider settings for each model. Kitsune Proxy handles endpoint selection and network forwarding, not protocol adaptation.

## Design principles

- **Transparent forwarding:** Preserve the HTTP method, path, body, response status, streaming behavior, and compatible headers.
- **Explicit routing:** Resolve each `model_id` against a configured upstream endpoint.
- **No API conversion:** Do not translate request or response schemas between OpenAI, Anthropic, or other provider protocols. The upstream must support the protocol sent by the Agent.
- **Local first:** Serve local Agents rather than operating as a public gateway or multi-tenant proxy.
- **Cross-platform:** Target Windows, Linux, and macOS with a native system tray entry point.

## Planned project layout

The Go modules are expected to be organized by responsibility:

- `cmd/kitsune-proxy/` — executable entry point.
- `internal/proxy/` — local HTTPS server and transparent forwarding.
- `internal/router/` — `model_id` to upstream configuration resolution.
- `internal/config/` — configuration loading, validation, and persistence.
- `internal/tray/` — tray UI and platform integration.
- `internal/app/` — application lifecycle and module coordination.
- `assets/` — platform icons and packaged resources.

See [AGENTS.md](AGENTS.md) for detailed code organization and contribution conventions.

## Development

The repository does not yet contain a Go module or implementation. After the initial scaffold is added, the expected standard Go workflow is:

```bash
go mod tidy                    # inferred from the planned Go toolchain in AGENTS.md
go run ./cmd/kitsune-proxy     # inferred from the planned Go toolchain in AGENTS.md
go test ./...                  # inferred from the planned Go toolchain in AGENTS.md
go test -race ./...            # inferred from the planned Go toolchain in AGENTS.md
go fmt ./...                   # inferred from the planned Go toolchain in AGENTS.md
go vet ./...                   # inferred from the planned Go toolchain in AGENTS.md
```

Network tests should use `httptest` and local fake upstreams; they must not contact real model providers. Cross-platform changes should validate builds for Windows, Linux, and macOS.

## Security considerations

- Listen on a loopback address by default; do not expose the service publicly by accident.
- Never commit provider tokens, private keys, real endpoint configurations, or user request data.
- Redact authorization headers, credentials, and sensitive request bodies from logs.
- Store local certificates, private keys, and configuration files with restricted permissions.

## Non-goals

- Converting requests or responses between model-provider API formats.
- Providing a hosted, multi-user, or multi-tenant proxy service.
- Changing the business semantics of Agent requests or hiding provider capability differences.

## Contributing

The project is still at an early stage. Read [AGENTS.md](AGENTS.md) before making changes. Pull requests should describe behavior changes, list verification commands, and identify the operating systems tested.
