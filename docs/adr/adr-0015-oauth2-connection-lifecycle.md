---
title: "ADR-0015: OAuth2 connection lifecycle for adapters"
status: "Proposed"
date: "2026-06-05"
authors: "Kaustav Das Modak, Claude"
tags: ["oauth", "connectors", "authentication", "ingestion"]
supersedes: ""
superseded_by: ""
---

# ADR-0015: OAuth2 connection lifecycle for adapters

## Status

**Proposed**

> Approved as design (`docs/design/2026-06-05-connector-adapterisation.md`); not yet
> implemented. Becomes **Accepted** when the OAuth flow ships in code (exercised by the
> Mastodon adapter).

## Context

Some integrations authenticate with an API key or a long-lived token
([[adr-0014-encrypted-credential-storage]] covers storing those), but others —
Mastodon/Fediverse and most modern services — require the **OAuth2 authorisation-code
flow**: redirect the user to the provider, exchange a returned code for access and
refresh tokens, and refresh the access token before it expires.

None of the first three validation integrations (RSS, Discourse, Matrix) uses OAuth, yet
the framework spine commits to shipping the OAuth machinery so it is not added later as a
breaking change. To avoid shipping untested machinery, a real OAuth integration
(Mastodon) is built alongside it as the exerciser. The flow must fit the
Connection/adapter model ([[adr-0013-adapterised-connectors-connection-entity]]) and the
HTTP conventions ([[adr-0006-http-api-conventions]]) rather than inventing a parallel
auth subsystem.

## Decision

Model OAuth as an **optional adapter capability plus a Connection lifecycle**, mirroring
how collect/publish/stream are optional roles.

- **Auth type**: `Connection.AuthType` ∈ {`none`, `api_key`, `token`, `oauth2`}. OAuth
  connections add the `pending_auth` status.
- **Optional role**: `core.OAuthProvider` — `AuthCodeURL(conn, redirectURI, state)`,
  `Exchange(ctx, conn, code, redirectURI)`, `Refresh(ctx, conn, refreshToken)` returning
  `OAuthTokens{AccessToken, RefreshToken, Expiry, Scopes}`. Only adapters whose
  `Capabilities().OAuth` is true implement it.
- **Lifecycle**:
  1. Create connection → `pending_auth`.
  2. `GET /api/v1/connections/{id}/authorize` → `AuthCodeURL` → `302` to the provider,
     carrying a short-lived **`state`** CSRF token bound to `(connection, owner)`.
  3. Provider redirects to `GET /api/v1/connections/oauth/callback?code&state` →
     validate `state` → `Exchange` → store tokens (encrypted, [[adr-0014-encrypted-credential-storage]]) →
     `active`, recording `CredentialExpiry`.
- **Lazy refresh**: before any adapter call, if `AuthType==oauth2` and the access token
  is near `CredentialExpiry`, call `Refresh` and persist the new tokens. No background
  refresher runner.

## Consequences

### Positive

- **POS-001**: OAuth reuses the same Connection entity, credential encryption, and HTTP
  conventions — no parallel auth subsystem.
- **POS-002**: OAuth is opt-in per adapter via a capability flag, so non-OAuth
  integrations carry none of its surface.
- **POS-003**: Shipping the flow with a real exerciser (Mastodon) means the
  authorize/callback/refresh paths are tested, not speculative.
- **POS-004**: Lazy refresh keeps the design stateless between calls — no always-on
  refresher loop to supervise.

### Negative

- **NEG-001**: The callback endpoint and `state` store add a stateful, browser-redirect
  surface (CSRF handling, redirect-URI registration) absent from key/token auth.
- **NEG-002**: Lazy refresh can add latency to the first adapter call after expiry, and a
  failed refresh must transition the Connection to `error` and surface re-authorisation.
- **NEG-003**: Building the full flow before more than one OAuth integration exists risks
  modelling to Mastodon's specifics; other providers may need follow-up generalisation.

## Alternatives Considered

### Seam now, flow later

- **ALT-001**: **Description**: Define `AuthType` + `OAuthProvider` in `core` now but
  defer the authorize/callback endpoints and refresh to a later sub-project.
- **ALT-002**: **Rejection Reason**: Leaves a declared-but-unusable capability; building
  the flow now with Mastodon as exerciser keeps the spine honest and tested.

### Out-of-band token paste

- **ALT-003**: **Description**: Have the user obtain a token externally and paste it as a
  `token` credential, skipping the redirect flow.
- **ALT-004**: **Rejection Reason**: Works for some providers but not those requiring
  authorisation-code grant with refresh; pushes provider-specific friction onto users and
  cannot refresh.

### Eager background token refresher

- **ALT-005**: **Description**: A runner that proactively refreshes all OAuth tokens
  before expiry.
- **ALT-006**: **Rejection Reason**: Another always-on loop to supervise for marginal
  benefit; lazy refresh on use covers the need with no extra runtime.

## Implementation Notes

- **IMP-001**: `state` is a signed/short-TTL token bound to `(connection_id, owner_id)`;
  store with an expiry and single-use semantics to resist CSRF/replay.
- **IMP-002**: The redirect URI is deployment configuration; document registering it with
  each provider.
- **IMP-003**: A failed `Refresh` sets the Connection to `error` with `LastError` and
  requires re-running `authorize`; surface this in the realtime/health path.

## References

- **REF-001**: [[adr-0013-adapterised-connectors-connection-entity]] — the Connection
  entity and adapter role interfaces.
- **REF-002**: [[adr-0014-encrypted-credential-storage]] — where access/refresh tokens
  are stored.
- **REF-003**: [[adr-0006-http-api-conventions]] — the API conventions the authorize and
  callback endpoints follow.
- **REF-004**: `docs/design/2026-06-05-connector-adapterisation.md` — §4.2 OAuth.
