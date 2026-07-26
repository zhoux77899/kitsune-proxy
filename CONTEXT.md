# Kitsune Proxy Domain Context

Kitsune Proxy is a single-user, loopback-only HTTP forwarding application. The
domain is one context: selecting a configured model route and transparently
forwarding an Agent request to its upstream.

## Core flow

1. An Agent sends a JSON request to the fixed loopback listener.
2. Kitsune Proxy authenticates supported inbound headers with the Local API Key.
3. The router resolves the exact Public Model Name from the unique top-level
   `model` string.
4. The proxy replaces the local base URL with the route's Upstream Base URL and
   appends the Agent request path.
5. The proxy replaces or removes supported authentication headers according to
   the route.
6. If the route uses a distinct Model Alias, the proxy minimally replaces it
   with the Upstream Model ID.
7. The upstream response is streamed back without schema conversion.

## Glossary

### Local API Key

The secret in `server.api_key`. An Agent uses it to authenticate to the local
listener. It is never an upstream credential and must never reach an upstream.

Avoid: proxy provider key, local provider key.

### Upstream API Key

The secret in `upstreams.<name>.auth.api_key`. With `auth.mode: replace`, it
replaces a validated Local API Key in the same inbound header format before
forwarding.

Avoid: local key, Agent key.

### Upstream

A named HTTP or HTTPS base URL, authentication rule, and non-empty set of model
routes. Its name is used as `owned_by` in `/v1/models`.

### Upstream Base URL

The absolute HTTP or HTTPS URL in `upstreams.<name>.url`. It may include a path,
such as `https://openrouter.ai/api`. The Agent request path is appended with one
joining slash, producing `https://openrouter.ai/api/v1/messages` for an inbound
`/v1/messages` request. Userinfo, queries, and fragments are not allowed.

### Upstream Model ID

The value in a model entry's `id`. It is the real identifier expected by the
selected Upstream.

Avoid: model alias, public model.

### Model Alias

The optional value in a model entry's `alias`. It distinguishes models exposed
by different Upstreams and becomes the Public Model Name when present.

Avoid: upstream model ID.

### Public Model Name

The case-sensitive, globally unique name accepted from the Agent and advertised
by `/v1/models`. It is the Model Alias when configured; otherwise it is the
Upstream Model ID.

Avoid: `model_id` when referring to the public routing value.

### Snapshot

An immutable, request-scoped set of Local API Key, model routes, Upstream API
Keys, and related forwarding settings. A request uses the Snapshot acquired at
its start even if Reload succeeds while it is in flight.

### Reload

A complete configuration transaction. Validation and any new listener binding
must succeed before the active Snapshot changes. Reload never applies only part
of a configuration.

### Deployment

One bound loopback listener and HTTP server associated with a validated
Snapshot. A port-changing Reload prepares a new Deployment before draining the
old one.

### Start-at-login preference

A per-user operating-system registration controlled by the tray. It is not part
of `config.yaml` and does not affect a Snapshot.

## Invariants

- The listener address is always `127.0.0.1`; only its port is configurable.
- Every Public Model Name resolves to exactly one Upstream Model ID.
- Authentication finishes before the business request body is read.
- A Local API Key never crosses the upstream seam.
- The outbound URL uses the selected Upstream Base URL followed by the Agent
  request path, preserving escaped path bytes and the Agent query.
- No route fallback, prefix match, glob, protocol conversion, or WebSocket
  forwarding exists.
- Logs never contain credentials, header values, query strings, bodies, or full
  configuration.
