# Usage Endpoint Dynamic Filtering — Implementation Plan

**Requirement**: [2026-04-20-usage-endpoint-filtering.md](2026-04-20-usage-endpoint-filtering.md)
**Date**: 2026-04-20
**Status**: Implemented

## Implementation Steps

1. **`usage.go`** — Rewrite `usageHandler` to parse path segments after `/usage` and return data at the matching nesting depth. Add a private `jsonNotFound` helper. Remove the `{"usage":...}` wrapper. Add `"net/url"` and `"strings"` imports.
2. **`main.go`** — Register the handler under both `/usage` (exact) and `/usage/` (subtree prefix) so sub-paths are routed to the usage handler, not the proxy.
3. **`proxy.go`** — Update `ModifyResponse` guard from `== "/usage"` to `strings.HasPrefix(path, "/usage")` for correctness (defensive; sub-paths already won't reach the proxy).
4. **`main_test.go`** — Update `TestUsageEndpoint_ReturnsJSON` to decode without the `"usage"` wrapper; update `buildTestServer` to register `/usage/`; add six new tests covering each URL variant and 404 paths.

## File Change Summary

| File | Action | Description |
|------|--------|-------------|
| `usage.go` | Modify | Replace handler with segment-parsing multi-level handler; add `jsonNotFound` |
| `main.go` | Modify | Register `/usage/` alongside `/usage` |
| `proxy.go` | Modify | Broaden `/usage` guard to `strings.HasPrefix` |
| `main_test.go` | Modify | Update existing test; add new sub-path and 404 tests |

## API Contracts

| Method | Path | Response shape | 404 trigger |
|--------|------|----------------|-------------|
| GET | `/usage` | `{ "<date>": { "<user>": { "<model>": {…stats} } } }` | never |
| GET | `/usage/{date}` | `{ "<user>": { "<model>": {…stats} } }` | date not in store |
| GET | `/usage/{date}/{user}` | `{ "<model>": {…stats} }` | date or user not in store |
| GET | `/usage/{date}/{user}/{model}` | `{…stats}` | date, user, or model not in store |
| GET | `/usage/{…>3 segments}` | HTTP 404 plain text | always |

404 body: `{"error":"not found"}` with `Content-Type: application/json`.

## Key Code Snippets

### `usage.go` — new handler

```go
func usageHandler(store *UsageStore) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        rest := strings.TrimPrefix(r.URL.Path, "/usage")
        rest = strings.Trim(rest, "/")
        var segments []string
        if rest != "" {
            segments = strings.Split(rest, "/")
        }
        for i, s := range segments {
            if dec, err := url.PathUnescape(s); err == nil {
                segments[i] = dec
            }
        }

        snapshot := store.Snapshot()
        var result any
        switch len(segments) {
        case 0:
            result = snapshot
        case 1:
            v, ok := snapshot[segments[0]]
            if !ok { jsonNotFound(w); return }
            result = v
        case 2:
            d, ok := snapshot[segments[0]]
            if !ok { jsonNotFound(w); return }
            v, ok := d[segments[1]]
            if !ok { jsonNotFound(w); return }
            result = v
        case 3:
            d, ok := snapshot[segments[0]]
            if !ok { jsonNotFound(w); return }
            u, ok := d[segments[1]]
            if !ok { jsonNotFound(w); return }
            v, ok := u[segments[2]]
            if !ok { jsonNotFound(w); return }
            result = v
        default:
            http.NotFound(w, r)
            return
        }

        data, _ := json.Marshal(result)
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        w.Write(data)
    }
}

func jsonNotFound(w http.ResponseWriter) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusNotFound)
    w.Write([]byte(`{"error":"not found"}`))
}
```

### `main.go` — route registration

```go
usageH := authMiddleware(validTokens, usageHandler(store))
mux.Handle("/usage", usageH)
mux.Handle("/usage/", usageH)
```

### `proxy.go` — broadened guard

```go
if strings.HasPrefix(resp.Request.URL.Path, "/usage") {
    return nil
}
```

## Unit Tests

| Test | URL / Input | Expected |
|------|-------------|----------|
| `TestUsageEndpoint_ReturnsJSON` (updated) | `GET /usage` (store has data) | 200, direct map (no `"usage"` wrapper) |
| `TestUsageEndpoint_FilterByDate` | `GET /usage/2026-04-16` | 200, user→model→stats map |
| `TestUsageEndpoint_FilterByDateUser` | `GET /usage/2026-04-16/good-token` | 200, model→stats map |
| `TestUsageEndpoint_FilterByDateUserModel` | `GET /usage/2026-04-16/good-token/llama3` | 200, stats object |
| `TestUsageEndpoint_NotFound_Date` | `GET /usage/1999-01-01` | 404, `{"error":"not found"}` |
| `TestUsageEndpoint_NotFound_User` | `GET /usage/2026-04-16/ghost` | 404, `{"error":"not found"}` |
| `TestUsageEndpoint_NotFound_Model` | `GET /usage/2026-04-16/good-token/ghost-model` | 404, `{"error":"not found"}` |
| `buildTestServer` (updated) | — | registers `/usage/` in addition to `/usage` |

## Risks & Open Questions

- **Breaking change**: `GET /usage` no longer wraps data in `{"usage":...}`. Existing clients break. Accepted per design approval.
- **Model names with `/`**: URL path `qwen3:0.5b` contains no slash; names with `/` would need `%2F` encoding. The `url.PathUnescape` call in the handler handles this correctly.
- **Go mux redirect**: Registering only `/usage/` without `/usage` would cause a 301 redirect on `GET /usage`. Registering both avoids this.
