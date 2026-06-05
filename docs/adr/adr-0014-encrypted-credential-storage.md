---
title: "ADR-0014: Encrypted credential storage with a SecretCipher port"
status: "Proposed"
date: "2026-06-05"
authors: "Kaustav Das Modak, Claude"
tags: ["security", "credentials", "connectors", "storage"]
supersedes: ""
superseded_by: ""
---

# ADR-0014: Encrypted credential storage with a SecretCipher port

## Status

**Proposed**

> Approved as design (`docs/design/2026-06-05-connector-adapterisation.md`); not yet
> implemented. Becomes **Accepted** when credential storage ships in code.

## Context

The Connection entity ([[adr-0013-adapterised-connectors-connection-entity]]) holds
secret material — Discourse API keys, Matrix access tokens, OAuth access/refresh tokens.
Storing these in the existing `Config map[string]any` would land them in SQLite in
plaintext and echo them back from `GET /connections`, leaking via API responses,
database backups, future multi-device sync, and logs.

Constraints: the system is local-first and frequently headless (containers, servers), so
credential protection must not require an interactive desktop. It must also remain
portable across the local (SQLite) / remote (Postgres) archive split
([[adr-0005-local-first-archive-storage-split]]) without re-keying, and crypto must stay
out of the `core` domain package (hexagonal isolation,
[[adr-0002-hexagonal-architecture-infrastructure-isolation]]).

## Decision

Make the secret/non-secret split **structural** and encrypt secrets at rest through a
`core` port.

- **Split**: `Connection.Config` (non-secret; returned in API) is separate from
  `Connection.Credentials` (secret; `json:"-"`; stored in a dedicated encrypted
  column/table). Adapters **declare** which fields are secret via `ConnectionSchema`
  (`FieldSpec{Name, Secret, Required, AuthType}`), so routing and redaction are
  data-driven, not a per-connector `switch`.
- **Port + implementation**: `core.SecretCipher` (`Encrypt`/`Decrypt`) is the domain
  port; the implementation lives outside `core` (a `secrets`/`crypto` package) using
  **AES-256-GCM**, and is injected into the archive. Ciphertext is what the archive
  persists; the cipher sits above storage, so encrypted credentials migrate across the
  local/remote archive split unchanged.
- **Key management**: the master key comes from **config/env** (`UPSPEAK_SECRET_KEY`).
  This works headless and in containers. If the key is missing when a credentialed
  Connection must be read or written, the operation **fails closed** rather than
  proceeding insecurely.
- **API + logging**: credentials are **write-only** on create/update; `GET` returns
  `Config` plus a `credential_status` (set/unset, optional last-4) but never the secret.
  Logs redact credential fields. Secrets are decrypted **transiently**, only when
  building a `*Connection` to hand to an adapter.

## Consequences

### Positive

- **POS-001**: Secrets are never stored in plaintext and never serialised to API
  responses; redaction is driven by adapter-declared schema, not ad-hoc code.
- **POS-002**: A config/env key works in headless and container deployments and keeps
  ciphertext portable across the local/remote archive split with no re-keying.
- **POS-003**: Crypto stays out of `core`; the domain depends only on a small port,
  preserving hexagonal isolation.
- **POS-004**: Fail-closed on a missing key prevents silent plaintext fallback.

### Negative

- **NEG-001**: Key management becomes the operator's responsibility; a lost
  `UPSPEAK_SECRET_KEY` makes stored credentials unrecoverable (re-authorise required).
- **NEG-002**: An env/config master key is weaker than an HSM/OS-keychain-protected key —
  it can leak via process environment, core dumps, or a misconfigured deployment.
- **NEG-003**: Encrypted credential columns are opaque to SQL — no querying, and key
  rotation requires a decrypt-re-encrypt migration pass.

## Alternatives Considered

### Plaintext now, encrypt later

- **ALT-001**: **Description**: Store credentials in `Config` initially and add
  encryption in a later phase.
- **ALT-002**: **Rejection Reason**: The first integrations (Discourse, Matrix) carry
  real secrets immediately; shipping plaintext even briefly is an avoidable leak and a
  breaking migration later.

### OS keychain / external secrets manager (Vault, etc.)

- **ALT-003**: **Description**: Keep secrets in the OS keychain or an external secrets
  backend; the DB stores only a reference.
- **ALT-004**: **Rejection Reason**: Platform-specific and awkward for headless/server
  and remote-archive deployments; adds an external dependency. The master key *may* later
  be sourced from a keychain while keeping ciphertext in the DB — an additive refinement,
  not the baseline.

### Master key from the OS keychain instead of config/env

- **ALT-005**: **Description**: AES-GCM in the DB, but load the master key from the OS
  keychain.
- **ALT-006**: **Rejection Reason**: Better key protection on a personal device, but
  breaks the headless/container story; deferred as an optional key-source upgrade behind
  the same `SecretCipher` port.

## Implementation Notes

- **IMP-001**: AES-256-GCM with a random per-record nonce stored alongside the
  ciphertext; key derived/validated from `UPSPEAK_SECRET_KEY` at startup.
- **IMP-002**: Key rotation is a maintenance operation (decrypt with old, re-encrypt with
  new); design the credential column to carry a key/version tag to support it.
- **IMP-003**: Audit all log sites and API serialisers for credential leakage; add tests
  asserting `GET /connections` never returns secret fields.

## References

- **REF-001**: [[adr-0013-adapterised-connectors-connection-entity]] — the Connection
  entity whose credentials this protects.
- **REF-002**: [[adr-0002-hexagonal-architecture-infrastructure-isolation]] — the
  `SecretCipher` port keeps crypto out of `core`.
- **REF-003**: [[adr-0005-local-first-archive-storage-split]] — ciphertext portability
  across local/remote archives.
- **REF-004**: `docs/design/2026-06-05-connector-adapterisation.md` — §4 credentials.
