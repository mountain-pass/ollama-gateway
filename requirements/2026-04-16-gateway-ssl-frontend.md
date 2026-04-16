# Gateway SSL (Frontend)

**Date Added**: 2026-04-16
**Priority**: High
**Status**: Planned

## Problem Statement

The gateway currently listens on plain HTTP. Clients connecting over untrusted networks need TLS so that API tokens and request payloads are not transmitted in cleartext.

## Functional Requirements

1. When `HTTPS=true`, the gateway starts an HTTPS listener using the certificate and private key supplied by `HTTPS_CERTIFICATE` and `HTTPS_PRIVATE_KEY`.
2. When `HTTPS` is absent or `false`, the gateway behaves exactly as today (plain HTTP).
3. If `HTTPS=true` and either `HTTPS_CERTIFICATE` or `HTTPS_PRIVATE_KEY` is missing, the process exits with a clear error message before binding.
4. The proxy logic, auth middleware, and `/usage` endpoint are unaffected; only the transport layer changes.

## User Experience Requirements

- Operators set three environment variables; no code changes or recompilation required.
- Default certificate and key paths are `/etc/ollama-gateway/cert.pem` and `/etc/ollama-gateway/key.pem` — sensible for a containerised deployment; operators can override with any path.
- The startup log clearly states whether the server is using HTTP or HTTPS.

## Technical Requirements

- Use `http.ListenAndServeTLS(addr, certFile, keyFile, handler)` — no new dependencies.
- `HTTPS` env var: optional, accepted values `true` / `false` (case-insensitive), default `false`.
- `HTTPS_CERTIFICATE` env var: path to PEM certificate file, default `/etc/ollama-gateway/cert.pem`.
- `HTTPS_PRIVATE_KEY` env var: path to PEM private key file, default `/etc/ollama-gateway/key.pem`.
- Only the frontend (client-facing) listener is affected; backend communication with Ollama remains plain HTTP.

## Acceptance Criteria

- [ ] `HTTPS=false` (or unset): server starts on plain HTTP, existing behaviour unchanged.
- [ ] `HTTPS=true` with valid cert and key paths: server starts on HTTPS and handles TLS connections.
- [ ] `HTTPS=true` with missing `HTTPS_CERTIFICATE` path value (no env var set): process exits with a descriptive error before binding.
- [ ] `HTTPS=true` with missing `HTTPS_PRIVATE_KEY` path value (no env var set): process exits with a descriptive error before binding.
- [ ] `HTTPS=true` with a non-existent or unreadable cert/key file: `ListenAndServeTLS` returns an error and the process exits non-zero.
- [ ] Startup log includes the scheme (`http` or `https`) in the listening message.
- [ ] README.md documents the three new env vars.

## Dependencies

- REQ-001 (Ollama HTTP Proxy Gateway) — this requirement extends the server startup in `main.go`.

## Implementation Notes

- Change is confined to `main.go` (env var reading + conditional `ListenAndServeTLS`) and `README.md`.
- `HTTPS` values other than `true` / `false` (case-insensitive) should be treated as `false` with no error, or exit with an error — to be decided during Plan phase.
