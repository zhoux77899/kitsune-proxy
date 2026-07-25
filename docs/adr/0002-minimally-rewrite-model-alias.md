# ADR 0002: Minimally rewrite a Model Alias

- Status: Accepted
- Date: 2026-07-25

## Context

Different Upstreams can expose the same Upstream Model ID. Exact routing needs a
globally unique Public Model Name, but sending that public alias unchanged would
ask the Upstream for a model it does not know.

## Decision

A model entry may define a Model Alias. The alias becomes its Public Model Name,
and the original `id` remains its Upstream Model ID.

When the two differ, Kitsune Proxy locates the unique top-level JSON `model`
string and replaces only that byte range. Identity bodies preserve every other
byte. Gzip bodies are bounded, decompressed, changed the same way, and
recompressed. Body-integrity headers cause a local 400 response because they
cannot remain valid after rewriting.

When the Public Model Name equals the Upstream Model ID, the request body bytes
remain unchanged.

## Consequences

- Models with identical provider IDs can be routed unambiguously.
- `/v1/models` exposes only Public Model Names.
- Kitsune Proxy remains schema-agnostic; it understands only the top-level
  routing field.
- This is intentional non-transparent behavior and must remain covered by
  byte-preservation, gzip, duplicate-field, and integrity-header tests.
