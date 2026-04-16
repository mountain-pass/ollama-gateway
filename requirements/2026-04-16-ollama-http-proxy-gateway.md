# Ollama HTTP Proxy Gateway

**Date Added**: 2026-04-16
**Priority**: High
**Status**: Planned

## Problem Statement

There is a need for a lightweight authenticated HTTP proxy that sits in front of a single Ollama instance. Clients should authenticate with API tokens, all proxied traffic should be HTTP (no TLS termination toward the backend), and token usage reported by Ollama should be tracked in memory per-token per-day and exposed through a dedicated endpoint.

## Functional Requirements

1. **Go project** — The service is implemented in Go (standard library preferred; minimal external dependencies).
2. **Authentication** — All incoming requests are rejected with `401 Unauthorized` unless they carry a valid `Authorization: Bearer <token>` header. Valid tokens are supplied via the `API_TOKENS` environment variable (comma-separated list).
3. **Reverse proxy** — All authenticated requests are forwarded verbatim to the backend Ollama instance specified by `OLLAMA_BASE_URL` over plain HTTP.
4. **Response inspection** — Every response body from the backend is inspected for Ollama usage fields (`prompt_tokens`, `eval_count`, `prompt_eval_count`) before being streamed back to the caller. Streaming (NDJSON) responses must be handled so each newline-delimited JSON object is inspected individually.
5. **Usage tracking** — Token usage and request counts are accumulated in memory, keyed by `(YYYY-MM-DD, api_token)`. The in-memory store is reset on service restart.
6. **Usage endpoint** — `GET /usage` is a reserved endpoint served by the proxy itself (not forwarded to Ollama). It returns a JSON summary of accumulated usage, grouped by date then API token.
7. **Environment variables**:
   - `OLLAMA_BASE_URL` — base URL of the backend Ollama instance (e.g. `http://localhost:11434`).
   - `API_TOKENS` — comma-separated list of valid bearer tokens (e.g. `token-a,token-b`).

## User Experience Requirements

- Clients interact with the proxy exactly as they would with the Ollama API — the only difference is the `Authorization` header requirement.
- The `/usage` endpoint is accessible only to clients that hold a valid API token (same 401 gate applies).
- Usage figures are human-readable JSON.

## Technical Requirements

- Go modules (`go.mod` / `go.sum`).
- No external router library required; `net/http` stdlib mux is sufficient.
- Proxy implementation via `httputil.ReverseProxy` or manual request forwarding; body must be re-readable for inspection without double-buffering the full response in memory for streaming payloads.
- Streaming responses: Ollama emits newline-delimited JSON for `/api/generate`, `/api/chat`, etc. The proxy must flush each chunk to the client immediately (no buffering) while inspecting each JSON object for usage fields.
- Usage fields to extract from each JSON object:
  - `prompt_eval_count` (prompt tokens)
  - `eval_count` (completion tokens)
  - The last object in a streaming response typically carries the final totals; accumulate only from objects where `done == true` or fall back to accumulating all values if `done` is absent.
- Thread-safe in-memory counters (mutex-protected `map[string]map[string]*UsageStat` where outer key is ISO date `YYYY-MM-DD` and inner key is API token).
- Each `UsageStat` entry tracks: `requests`, `prompt_tokens`, `completion_tokens`, `total_tokens`.
- Request count is incremented once per proxied request (regardless of whether the response contains usage fields).
- Service should start on port `8080` by default; optionally configurable via `PORT` env var.

## Acceptance Criteria

- [ ] Service starts and listens on the configured port.
- [ ] Request without `Authorization` header returns `401 Unauthorized`.
- [ ] Request with an invalid token returns `401 Unauthorized`.
- [ ] Request with a valid token is proxied to `OLLAMA_BASE_URL` unchanged.
- [ ] Non-streaming Ollama responses have their usage fields captured and added to the in-memory counter.
- [ ] Streaming Ollama responses are flushed to the client chunk-by-chunk; the final `done: true` object's usage fields are captured.
- [ ] `GET /usage` (with valid auth) returns JSON with per-date, per-token usage totals including request counts.
- [ ] `GET /usage` (without valid auth) returns `401 Unauthorized`.
- [ ] Service reads `OLLAMA_BASE_URL` and `API_TOKENS` from environment at startup; exits with an informative error if either is missing.
- [ ] `go build` produces a single static binary with no CGO dependencies.

## Dependencies

None — this is a greenfield Go service.

## Implementation Notes

- Use `httputil.NewSingleHostReverseProxy` as a starting point but override the `ModifyResponse` hook to inspect and wrap the response body for usage extraction.
- For streaming bodies, wrap `http.ResponseWriter` with a custom writer that intercepts writes, splits on newlines, and parses each JSON fragment.
- The `/usage` JSON shape (proposed):

```json
{
  "usage": {
    "2026-04-16": {
      "token-a": {
        "requests": 5,
        "prompt_tokens": 120,
        "completion_tokens": 340,
        "total_tokens": 460
      }
    }
  }
}
```
