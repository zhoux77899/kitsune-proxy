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
  |  POST /v1/messages
  |  {"model":"team-chat", ...}
  v
http://127.0.0.1:18080/v1/messages
  |  exact alias lookup
  |  local key -> upstream key
  |  "team-chat" -> "chat-model-v2"
  v
https://upstream.example.com/api/v1/messages
```

Only two operations intentionally change an otherwise transparent request:

1. The Agent's Local API Key is replaced with the selected Upstream API Key.
2. A Model Alias is minimally rewritten to the corresponding Upstream Model ID.

Method, Agent path suffix, query, compatible headers, status, response bytes,
SSE flushing, and cancellation are preserved. The selected Upstream Base URL
replaces the local base URL before the Agent path is appended. Kitsune Proxy
does not convert provider API formats, so the selected upstream must understand
the Agent's request schema and authentication header type.

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
  hosted-service:
    url: https://upstream.example.com/api
    auth:
      mode: replace
      api_key: upstream-secret
    models:
      - id: chat-model-v2
        alias: team-chat

  secondary-service:
    url: https://secondary.example.com/gateway
    auth:
      mode: replace
      api_key: secondary-secret
    models:
      - id: reasoning-model-v1

  local-service:
    url: http://127.0.0.1:11434
    auth:
      mode: none
    models:
      - id: local-model-v1
        alias: local-chat
```

Never reuse an upstream credential as `server.api_key`:

- `server.api_key` is the **Local API Key** given to Agents.
- `upstreams.<name>.auth.api_key` is an **Upstream API Key** stored locally and
  sent only to that upstream.

`auth.mode: replace` requires an upstream key. `auth.mode: none` forbids a key
and removes both supported inbound authentication headers before forwarding.
An upstream `url` is an absolute HTTP or HTTPS base URL. It may include a path,
but not userinfo, a query, or a fragment. Kitsune Proxy joins that base path to
the Agent path with one slash: `https://upstream.example.com/api` becomes
`https://upstream.example.com/api/v1/messages`, while
`https://secondary.example.com/gateway` becomes
`https://secondary.example.com/gateway/v1/messages`. Consult your upstream
service's documentation for its compatible base URL, request schema, and
authentication requirements.

### TLS verification

HTTPS Upstreams verify the certificate chain, SAN, and hostname by default. For
a trusted internal HTTPS service whose self-signed or internal-CA certificate
cannot be verified, verification can be disabled for that Upstream only:

```yaml
upstreams:
  internal-service:
    url: https://10.0.0.1:8443/api
    tls:
      skip_verify: true
    auth:
      mode: none
    models:
      - id: internal-model
```

`skip_verify: true` is accepted only for an `https://` URL and applies to every
model configured under that Upstream. It disables certificate-chain, SAN, and
hostname verification; it does not provide a CN fallback. Use it only when the
network and upstream are independently trusted. Omitting `tls`, using an empty
`tls: {}` block, or setting `skip_verify: false` keeps strict verification.

### Models and aliases

`id` is the upstream's real model identifier. `alias`, when present, is the
Public Model Name accepted by Kitsune Proxy and returned from `/v1/models`.
Without an alias, the public name equals `id`.

Public names are case-sensitive and globally unique. If two providers expose
the same model ID, give at least the conflicting entries distinct aliases:

```yaml
models:
  - id: chat-model-v2
    alias: service-a-chat
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
  -d '{"model":"team-chat","input":"hello"}'
```

Request bodies are limited to 64 MiB before and after gzip decompression.
Unknown models return 404; no fallback provider is selected.

## Install

Download packages from the [GitHub Releases](https://github.com/zhoux77899/kitsune-proxy/releases)
page. Verify downloads with `SHA256SUMS` when integrity matters.

### Windows

Download `kitsune-proxy-windows-amd64.exe` to run the application directly, or
use `kitsune-proxy-windows-amd64.zip` when you also want the README and license.

### Linux

Debian and Ubuntu users can install the DEB package:

```bash
sudo apt install ./kitsune-proxy-linux-amd64.deb
```

Fedora and other RPM-based distributions can install the RPM package:

```bash
sudo dnf install ./kitsune-proxy-linux-amd64.rpm
```

Both system packages add Kitsune Proxy to the desktop application menu. They do
not start it automatically; launch it once and use the tray's **Start at Login**
preference if desired. Other amd64 distributions can extract
`kitsune-proxy-linux-amd64.tar.gz` and run the executable inside it.

### macOS

Choose `amd64` for an Intel Mac or `arm64` for Apple silicon. The ZIP contains
`Kitsune Proxy.app` directly. The DMG presents the same application bundle with
an Applications shortcut for drag-and-drop installation.

The macOS application is not signed or notarized. If Gatekeeper blocks the
first launch, Control-click the application, choose **Open**, and confirm it.

`latest.json` is a versioned update-manifest contract. It points to one
canonical ZIP or tarball per platform and reserves an empty `signature` string
for future Ed25519/Minisign verification. Empty signatures must never be
accepted by a future updater; automatic updates are not implemented yet.

## Tray and reload

The tray menu shows listener state, address, model count, and logging
availability. It can open the configuration or log directory, reload the
configuration, enable or disable **Start at Login**, and quit.

On Windows, a solid dot at the lower-right of the tray icon shows service
health: green is healthy, yellow is starting or degraded, red means no listener
is available, and gray is stopped. On macOS, the menu bar uses a monochrome
template icon without a visible title or tooltip; macOS chooses black or white
for the current menu bar appearance. Linux keeps the color icon and title.

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
reproducibly creates the color Linux tray PNG, the base Windows ICO and macOS
ICNS application icons, four multi-resolution Windows status tray ICOs, and
black/white macOS template previews. The runtime embeds the black macOS template
and lets the operating system recolor it. The generator also creates the Windows
amd64 COFF resource containing the base application icon and a per-monitor V2
DPI manifest. Go automatically links the generated `rsrc_windows_amd64.syso`
resources into Windows amd64 executables.

Windows release builds use `-ldflags "-H=windowsgui"`. macOS releases package
the generated ICNS in unsigned ZIP and DMG `.app` bundles with `LSUIElement=1`.
Linux releases include a portable tarball plus DEB and RPM packages with a
desktop entry. Release assets never expose bare Linux or macOS executables. The
project intentionally does not include Apple signing, notarization, or active
automatic updates.

## Security notes

- The listener cannot be configured away from IPv4 loopback.
- TLS verification is enabled by default and can be disabled only for an
  explicitly configured HTTPS Upstream.
- Configuration and log files are created with user-only permissions on Unix.
- The configuration contains upstream secrets in plaintext; protect the user
  profile and never commit a real configuration.
- Local HTTP is intended only for same-machine Agents. Do not bridge or publish
  the port.

See [CONTEXT.md](CONTEXT.md) for the domain vocabulary and
[docs/adr](docs/adr) for intentional non-transparent behavior.
