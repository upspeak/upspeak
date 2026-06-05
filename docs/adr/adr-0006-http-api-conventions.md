---
title: "ADR-0006: HTTP API conventions — response envelope, flat URLs, ref resolution"
status: "Accepted"
date: "2026-06-05"
authors: "Kaustav Das Modak, Claude"
tags: ["api", "routing"]
supersedes: ""
superseded_by: ""
---

# ADR-0006: HTTP API conventions — response envelope, flat URLs, ref resolution

## Status

**Accepted**

## Context

Because Upspeak is API-first ([[adr-0001-api-first-server]]), the HTTP contract is the
product's primary surface and must be consistent across many modules and many entity
types. Clients need predictable response shapes, URLs that are easy to construct and
read, and the ability to reference entities by whatever identifier they have to hand
(UUID, human short ID, or a repository's slug). Several modules contribute handlers to
the same `/api/v1` mount, so the conventions must compose without collisions.

## Decision

Standardise three conventions across the API:

1. **Response envelope** — every response is `{"data": ..., "meta": {...},
   "error": {...}}`, written through `api.WriteJSON`, `api.WriteList`, and
   `api.WriteError` so shape and error coding are uniform.
2. **Flat URL routing** — entities are addressed at
   `/api/v1/repos/{repo_ref}/{entity_ref}`, where the **short-ID prefix encodes the
   entity type** (`NODE-…`, `EDGE-…`, …). Collection endpoints use typed segments
   (`/nodes`, `/edges`, `/threads`, `/annotations`), which are checked before the flat
   dispatch. Pagination is `?limit=&offset=&sort_by=&order=`.
3. **Ref resolution** — `{repo_ref}` accepts a UUID, short ID, or slug; `{entity_ref}`
   accepts a UUID or short ID. Renamed repositories keep their old slugs, which return
   **301** redirects to the canonical URL.

## Consequences

### Positive

- **POS-001**: Uniform envelope means clients parse one response shape and one error
  format everywhere.
- **POS-002**: Prefix-encoded short IDs let a single flat route resolve any entity type,
  keeping URLs short and human-usable (leverages [[adr-0004-identity-uuidv7-short-ids]]).
- **POS-003**: Multi-identifier ref resolution makes the API forgiving — UUID for
  machines, short ID for humans, slug for shareable repo URLs — and 301s keep old links
  working after a rename.
- **POS-004**: Modules co-mount cleanly because typed collection paths are matched
  before the catch-all flat dispatch (`http.ServeMux` specificity).

### Negative

- **NEG-001**: Flat dispatch keyed on the short-ID prefix needs a reserved-segment list
  (`nodes`, `edges`, …) checked first; adding a new top-level segment requires care to
  avoid shadowing the flat route.
- **NEG-002**: Ref resolution adds a lookup (and possible redirect) on the hot path of
  most requests.
- **NEG-003**: The convention is not universal — the rules and connector modules
  deliberately use **typed self-contained URLs** rather than the flat
  prefix-dispatch scheme, so the API has two URL idioms a reader must learn.

## Alternatives Considered

### Bare resources (no envelope)

- **ALT-001**: **Description**: Return entity JSON directly with no wrapper.
- **ALT-002**: **Rejection Reason**: No consistent place for pagination metadata or
  structured errors; clients special-case per endpoint.

### Type-segmented URLs everywhere (`/repos/{r}/nodes/{id}`)

- **ALT-003**: **Description**: Always include the entity type in the path for item
  endpoints, not just collections.
- **ALT-004**: **Rejection Reason**: More verbose and redundant given the short-ID
  prefix already encodes the type; the flat form is shorter and still unambiguous.
  (The rules/connector modules opt into typed URLs where their resources warrant it —
  NEG-003.)

## Implementation Notes

- **IMP-001**: Envelope and helpers in `api/envelope.go`, `api/http.go`; middleware
  (RequestID, security headers, ETag handling) in `api/middleware.go`.
- **IMP-002**: Flat dispatch and ref resolution in `repo/handlers_entity.go`,
  `repo/handlers_repo.go`; repository ref resolution via `RepositoryStore.ResolveRepoRef`
  and entity refs via `RefResolver.ResolveRef`.
- **IMP-003**: The typed-self-contained-URL divergence (rules, connector) is intentional
  and documented; keep new modules on the flat convention unless there is a specific
  reason to diverge.

## References

- **REF-001**: [[adr-0001-api-first-server]] — the API is the product surface.
- **REF-002**: [[adr-0004-identity-uuidv7-short-ids]] — short-ID prefixes power the flat
  routing.

> The optimistic-concurrency record (`adr-0007-optimistic-concurrency-version-etag.md`)
> builds on these conventions, defining the `ETag`/`If-Match` semantics; it references
> this ADR rather than the reverse.
