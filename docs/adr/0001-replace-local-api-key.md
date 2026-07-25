# ADR 0001: Replace the Local API Key at the upstream seam

- Status: Accepted
- Date: 2026-07-25

## Context

Agents need one credential for the local Kitsune Proxy listener, while each
configured Upstream may require a different secret. Transparently forwarding
the inbound credential would leak the Local API Key and would not authenticate
to the selected Upstream.

## Decision

Kitsune Proxy authenticates `Authorization: Bearer` and `x-api-key` with the
Local API Key before reading the business body. A route with
`auth.mode: replace` substitutes the selected Upstream API Key only in the
header format or formats supplied by the Agent. A route with `auth.mode: none`
removes both supported headers.

Kitsune Proxy does not synthesize or convert authentication header formats.
Unsupported Authorization schemes are rejected instead of forwarded.

## Consequences

- The Local API Key never reaches an Upstream.
- Agents can keep one local credential while routes use separate Upstream API
  Keys.
- An Upstream must accept the authentication header format chosen by the Agent.
- This is intentional non-transparent behavior and must remain covered by
  forwarding and credential-redaction tests.
