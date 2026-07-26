<p align="center">
  <img src="assets/kitsune.svg" alt="Kitsune Proxy fox icon" width="160">
</p>

<h1 align="center">
  Kitsune Proxy
</h1>

<p align="center">
  <a href="https://github.com/zhoux77899/kitsune-proxy/actions/workflows/ci.yml">
    <img src="https://img.shields.io/github/actions/workflow/status/zhoux77899/kitsune-proxy/ci.yml?branch=main&amp;label=CI&amp;logo=github&amp;style=flat-square" alt="CI status">
  </a>
  <img src="https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&amp;logoColor=white&amp;style=flat-square" alt="Go 1.25 or newer">
  <img src="https://img.shields.io/badge/platform-Windows%20%7C%20Linux%20%7C%20macOS-5C5C5C?style=flat-square" alt="Windows, Linux, and macOS">
  <a href="LICENSE">
    <img src="https://img.shields.io/badge/license-MIT-brightgreen?style=flat-square" alt="MIT license">
  </a>
  <img src="https://img.shields.io/badge/network-loopback%20only-F17C17?style=flat-square" alt="Loopback network only">
</p>

Kitsune Proxy is a personal, cross-platform system-tray application that
aggregates model-provider endpoints behind one loopback HTTP address. It routes
each JSON request by its top-level `model`, replaces the local credential with
the selected upstream credential, and streams the upstream response back
without protocol conversion.

The listener is always `127.0.0.1`. Kitsune Proxy does not provide a public
listener, TLS termination, a settings window, WebSocket forwarding, or schema
translation.

## Request flow

```text
Agent
  |  Authorization: Bearer <Local API Key>
  |  {"model":"openai-gpt-5", ...}
  v
http://127.0.0.1:18080
  |  exact alias lookup
  |  local key -> upstream key
  |  "openai-gpt-5" -> "gpt-5"
  v
https://api.openai.com
```

Only two operations intentionally change an otherwise transparent request:

1. The Agent's Local API Key is replaced with the selected Upstream API Key.
2. A Model Alias is minimally rewritten to the corresponding Upstream Model ID.

Method, path, query, compatible headers, status, response bytes, SSE flushing,
and cancellation are preserved. Kitsune Proxy does not convert provider API
formats, so the selected upstream must understand the Agent's request schema
and authentication header type.

## Configuration

Configuration has one fixed location:

- Windows: `%USERPROFILE%\.kitsune\config.yaml`
- Linux and macOS: `~/.kitsune/config.yaml`

There is no command-line, environment-variable, or alternate-path override.
The first start creates the directory and a valid empty configuration, including
a random 256-bit Local API Key.

```yaml
version: 1

server:
  port: 18080
  api_key: kitsune-generated-local-key

logging:
  level: info

upstreams:
  openai:
    url: https://api.openai.com
    auth:
      mode: replace
      api_key: sk-upstream-key
    models:
      - id: gpt-5
        alias: openai-gpt-5
      - id: text-embedding-3-small

  ollama:
    url: http://127.0.0.1:11434
    auth:
      mode: none
    models:
      - id: llama3.3
        alias: local-llama
```

Never reuse an upstream credential as `server.api_key`:

- `server.api_key` is the **Local API Key** given to Agents.
- `upstreams.<name>.auth.api_key` is an **Upstream API Key** stored locally and
  sent only to that upstream.

`auth.mode: replace` requires an upstream key. `auth.mode: none` forbids a key
and removes both supported inbound authentication headers before forwarding.
Upstream URLs must be HTTP or HTTPS origins without userinfo, non-root paths,
queries, or fragments.

### Models and aliases

`id` is the upstream's real model identifier. `alias`, when present, is the
Public Model Name accepted by Kitsune Proxy and returned from `/v1/models`.
Without an alias, the public name equals `id`.

Public names are case-sensitive and globally unique. If two providers expose
the same model ID, give at least the conflicting entries distinct aliases:

```yaml
models:
  - id: gpt-5
    alias: provider-a-gpt-5
```

When alias and ID differ, Kitsune Proxy changes only the unique top-level JSON
`model` string. Identity bodies retain all other bytes, whitespace, field order,
and number formatting. Gzip bodies are bounded, decompressed, minimally
rewritten, and recompressed. Requests that combine an alias rewrite with
`Content-MD5`, `Digest`, or `Content-Digest` are rejected because their body
signature would no longer be valid.

## Agent authentication

`GET /healthz` is public. `/v1/models` and all forwarded requests require one or
both of:

```http
Authorization: Bearer <server.api_key>
x-api-key: <server.api_key>
```

If both are present, both must match. Other Authorization schemes are rejected.
For `auth.mode: replace`, Kitsune Proxy preserves the Agent's chosen header
type: Bearer remains Bearer, `x-api-key` remains `x-api-key`, and both remain
both. It never converts between the two formats.

Examples:

```bash
curl http://127.0.0.1:18080/v1/models \
  -H "Authorization: Bearer kitsune-generated-local-key"

curl http://127.0.0.1:18080/v1/responses \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer kitsune-generated-local-key" \
  -d '{"model":"openai-gpt-5","input":"hello"}'
```

Request bodies are limited to 64 MiB before and after gzip decompression.
Unknown models return 404; no fallback provider is selected.

## Tray and reload

The tray menu shows listener state, address, model count, and logging
availability. It can open the configuration or log directory, reload the
configuration, enable or disable **Start at Login**, and quit.

Start-at-login is stored as a per-user operating-system preference:

- Windows: the current user's `Run` registry key.
- Linux: an XDG Autostart desktop entry.
- macOS: a user LaunchAgent.

Reload validates the complete file before applying anything. With an unchanged
port, new requests atomically receive the new authentication, routing,
credentials, and log level snapshot. With a changed port, Kitsune Proxy binds
the new loopback listener first and only then drains the old listener. Any
failure keeps the complete previous snapshot. Updating `server.api_key` takes
effect immediately after a successful reload; already authenticated in-flight
requests continue on their original snapshot.

An invalid configuration at startup leaves the tray available but does not
start a listener. Fix the file and choose **Reload Configuration**.

## Logs

Logs are written only to:

- Windows: `%USERPROFILE%\.kitsune\logs\`
- Linux and macOS: `~/.kitsune/logs/`

The active file is `kitsune.log`. At 10 MiB it rotates through
`kitsune.log.1` to `kitsune.log.5` without compression. Log levels are `debug`,
`info`, `warn`, and `error`.

Logs contain fixed English event names and safe request summaries. They never
contain query strings, header values, Local API Keys, Upstream API Keys, request
or response bodies, or full configuration. If the fixed log directory is not
writable, the proxy continues and the tray reports **Logging unavailable**.

## Build and test

The module compatibility baseline is Go 1.25. CI uses Go 1.26.

```bash
go mod tidy
go run ./cmd/icon-gen
go run ./cmd/kitsune-proxy
go test ./...
go test -race ./...
go vet ./...
go fmt ./...
```

`assets/kitsune.svg` is the only icon source. `go run ./cmd/icon-gen`
reproducibly creates the embedded tray PNG and ICO, the macOS ICNS, and the
Windows amd64 COFF resource containing the application icon and a per-monitor
V2 DPI manifest. Go automatically links the generated
`rsrc_windows_amd64.syso` resources into Windows amd64 executables.

Windows release builds use `-ldflags "-H=windowsgui"`. macOS releases package
the generated ICNS in unsigned `.app` bundles with `LSUIElement=1`. Linux
releases remain raw ELF binaries without desktop-entry packaging. The project
intentionally does not include installers, signing, notarization, or automatic
updates.

## Security notes

- The listener cannot be configured away from IPv4 loopback.
- Configuration and log files are created with user-only permissions on Unix.
- The configuration contains upstream secrets in plaintext; protect the user
  profile and never commit a real configuration.
- Local HTTP is intended only for same-machine Agents. Do not bridge or publish
  the port.

See [CONTEXT.md](CONTEXT.md) for the domain vocabulary and
[docs/adr](docs/adr) for intentional non-transparent behavior.
