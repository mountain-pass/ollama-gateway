# Ollama HTTP Proxy Gateway — Implementation Plan

**Requirement**: [2026-04-16-ollama-http-proxy-gateway.md](2026-04-16-ollama-http-proxy-gateway.md)
**Date**: 2026-04-16
**Status**: Draft

## Implementation Steps

1. **Initialise Go module** — run `go mod init github.com/nickgrealy/ollama-gateway` in the repo root, producing `go.mod`. No external dependencies; stdlib only.

2. **Create `main.go`** — entry point that:
   - Reads and validates `OLLAMA_BASE_URL` and `API_TOKENS` env vars; calls `log.Fatal` with a clear message if either is absent.
   - Parses `API_TOKENS` into a `map[string]struct{}` for O(1) lookup.
   - Reads optional `PORT` env var (default `"8080"`).
   - Constructs the usage store, proxy handler, and HTTP mux.
   - Calls `http.ListenAndServe`.

3. **Create `store.go`** — thread-safe usage store:
   - Type `UsageStat` struct with fields `Requests`, `PromptTokens`, `CompletionTokens`, `TotalTokens` (all `int64`).
   - Type `UsageStore` struct containing a `sync.Mutex` and `data map[string]map[string]*UsageStat` (outer key: ISO date, inner key: API token).
   - Method `Record(date, token string, prompt, completion int64)` — locks, lazily initialises nested maps, increments all counters.
   - Method `Snapshot() map[string]map[string]UsageStat` — locks, returns a deep copy safe for JSON marshalling.

4. **Create `auth.go`** — authentication middleware:
   - Function `authMiddleware(validTokens map[string]struct{}, next http.Handler) http.Handler`.
   - Parses `Authorization` header; splits on `" "` and checks `parts[0] == "Bearer"` and `parts[1]` is in `validTokens`.
   - Returns `401` with `WWW-Authenticate: Bearer` on failure.
   - On success, stores the token in the request context (`context.WithValue`) under a package-level key so downstream handlers can retrieve it without re-parsing.

5. **Create `proxy.go`** — reverse proxy with usage inspection:
   - Constructs a `*url.URL` from `OLLAMA_BASE_URL`.
   - Creates `httputil.NewSingleHostReverseProxy(target)`.
   - Overrides `proxy.ModifyResponse` with a function that:
     - Retrieves the API token from request context.
     - Gets today's ISO date string (`time.Now().UTC().Format("2006-01-02")`).
     - Increments the request counter unconditionally (`store.Record(date, token, 0, 0)` then separately add token counts, or accept a `requests` increment in the same call — see store design).
     - Wraps `resp.Body` with an `inspectingReader` (see step 6).
   - Sets `proxy.Director` to strip the `Authorization` header before forwarding (Ollama does not need it).

6. **Create `inspect.go`** — streaming-safe body inspector:
   - Type `inspectingReader` implements `io.ReadCloser`; wraps the original body.
   - Buffers partial reads; on each `Read` call, scans the accumulated buffer for complete newline-terminated JSON objects.
   - For each complete JSON object, attempts to unmarshal into a minimal struct:
     ```go
     type ollamaUsage struct {
         Done            bool  `json:"done"`
         PromptEvalCount int64 `json:"prompt_eval_count"`
         EvalCount       int64 `json:"eval_count"`
     }
     ```
   - If `done == true` (or if there is no `done` field at all and the object contains non-zero counts), records `prompt_eval_count` and `eval_count` into the store.
   - Passes bytes through to the caller unmodified.
   - Non-NDJSON responses (e.g. single-object JSON blobs): the same logic works — the single object is terminated by EOF; handle by flushing remaining buffer on `Close`.

7. **Create `usage.go`** — `/usage` HTTP handler:
   - `usageHandler(store *UsageStore) http.HandlerFunc`.
   - Calls `store.Snapshot()`, marshals to JSON with `json.Marshal`, writes `Content-Type: application/json` and `200 OK`.

8. **Wire up `main.go`** — register routes on `http.ServeMux`:
   - `GET /usage` → `usageHandler` (wrapped in `authMiddleware`).
   - All other paths → reverse proxy handler (wrapped in `authMiddleware`).
   - Note: `/usage` must be registered before the catch-all.

9. **Add `Makefile`** with targets `build`, `run`, `test`.

10. **Write `main_test.go`** — table-driven integration tests using `httptest`:
    - Missing auth → 401.
    - Bad token → 401.
    - Valid token, non-streaming mock backend → 200, usage recorded.
    - Valid token, streaming mock backend (NDJSON) → 200, chunks flushed, usage recorded from `done:true` object.
    - `/usage` without auth → 401.
    - `/usage` with auth → 200, correct JSON shape.

## File Change Summary

| File | Action | Description |
|------|--------|-------------|
| `go.mod` | Create | Go module declaration, no external deps |
| `main.go` | Create | Entry point: env parsing, wiring, server start |
| `store.go` | Create | Thread-safe in-memory usage store |
| `auth.go` | Create | Bearer token auth middleware + context key |
| `proxy.go` | Create | Reverse proxy construction and ModifyResponse hook |
| `inspect.go` | Create | Streaming-safe NDJSON body inspector |
| `usage.go` | Create | `/usage` endpoint handler |
| `main_test.go` | Create | Integration tests via httptest |
| `Makefile` | Create | `build`, `run`, `test` targets |

## API Contracts

### All proxied endpoints
- **Auth**: `Authorization: Bearer <token>` required on every request.
- **Forwarded to**: `OLLAMA_BASE_URL + original path + query string`.
- **Authorization header**: stripped before forwarding to Ollama.
- **Response**: passed through verbatim (headers + body).

### `GET /usage`
- **Auth**: same Bearer gate.
- **Response `200 OK`**:
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
- `total_tokens` = `prompt_tokens` + `completion_tokens`.

## Data Models

```
UsageStore
  mu   sync.Mutex
  data map[isodate string]
         map[api_token string]
           *UsageStat{
               Requests         int64
               PromptTokens     int64
               CompletionTokens int64
               TotalTokens      int64
             }
```

## Key Code Snippets

### Store.Record
```go
func (s *UsageStore) Record(date, token string, prompt, completion int64) {
    s.mu.Lock()
    defer s.mu.Unlock()
    if s.data[date] == nil {
        s.data[date] = make(map[string]*UsageStat)
    }
    st := s.data[date][token]
    if st == nil {
        st = &UsageStat{}
        s.data[date][token] = st
    }
    st.Requests++
    st.PromptTokens += prompt
    st.CompletionTokens += completion
    st.TotalTokens += prompt + completion
}
```

### inspectingReader.Read (outline)
```go
func (r *inspectingReader) Read(p []byte) (int, error) {
    n, err := r.body.Read(p)
    r.buf = append(r.buf, p[:n]...)
    r.scanLines()   // parse complete \n-terminated objects, call store.Record for done:true
    return n, err
}

func (r *inspectingReader) Close() error {
    r.scanLines()   // flush any remaining bytes (non-streaming single-object response)
    return r.body.Close()
}
```

### ModifyResponse hook
```go
proxy.ModifyResponse = func(resp *http.Response) error {
    token := resp.Request.Context().Value(ctxKeyToken).(string)
    date  := time.Now().UTC().Format("2006-01-02")
    store.RecordRequest(date, token)   // always increment request count
    resp.Body = &inspectingReader{
        body:  resp.Body,
        store: store,
        date:  date,
        token: token,
    }
    return nil
}
```

> **Note**: `RecordRequest` increments only `Requests`; `inspectingReader` calls a separate `RecordUsage(date, token, prompt, completion)` when it sees `done:true`. This keeps request counting independent of whether the response has usage fields.

## Unit Tests

| Test | Input | Expected Output |
|------|-------|-----------------|
| No Authorization header | `GET /api/tags` | 401 |
| Wrong scheme (`Token abc`) | `GET /api/tags` | 401 |
| Unknown token | `Authorization: Bearer bad` | 401 |
| Valid token, backend 200 | `GET /api/tags` valid auth | 200, body proxied |
| Non-streaming usage capture | Backend returns `{"done":true,"prompt_eval_count":10,"eval_count":20}` | store has prompt=10, completion=20, total=30 |
| Streaming usage capture | Backend streams 3 NDJSON lines, last has `done:true` with counts | store captures only last line's counts |
| `/usage` no auth | `GET /usage` | 401 |
| `/usage` valid auth | `GET /usage` after above request | 200, JSON contains date→token→counts |
| Request counter | 3 proxied requests with same token/date | `requests == 3` in `/usage` |

## Risks & Open Questions

- **Streaming flush**: `httputil.ReverseProxy` buffers by default. Need to ensure the `ResponseWriter` is flushed after each chunk. The proxy's `FlushInterval` field should be set to `-1` (flush immediately) to support streaming.
- **Token accumulation strategy**: The design records usage only from `done:true` objects. If a client disconnects mid-stream before `done:true` arrives, those tokens are not recorded. This is acceptable for an MVP.
- **Date boundary**: Requests use `time.Now().UTC()` at response time. A request that starts just before midnight and completes just after will be attributed to the completion date. Acceptable.
