# Track API Usage Metrics by Date / API Key / Model — Implementation Plan

**Requirement**: [2026-04-17-track-api-usage-metrics.md](2026-04-17-track-api-usage-metrics.md)
**Date**: 2026-04-17
**Status**: Implemented

## Implementation Steps

1. **`store.go`** — Replace `UsageStat` struct with new fields; change store map to three levels; rewrite `RecordRequest`, replace `RecordUsage` with `RecordResponse`, update `Snapshot` and `getOrCreate`.

2. **`proxy.go`** — Add a `ctxKeyModel` context key; in `Director` read the request body to extract the `"model"` field, restore the body, and inject the model string into the request context; in `ModifyResponse` read the model from context and pass it (plus `timestamp`) to `RecordRequest` and `newInspectingReader`.

3. **`inspect.go`** — Extend `ollamaUsage` with the four new timing fields; add `model` field to `inspectingReader`; update constructor; change `tryRecord` to call `store.RecordResponse` with the full `ollamaUsage` value.

4. **`main_test.go`** — Update all existing tests for the new struct field names and three-level snapshot shape; add five new test cases (see Unit Tests section).

## File Change Summary

| File | Action | Description |
|------|--------|-------------|
| `store.go` | Modify | New `UsageStat`, three-level map, new method signatures |
| `proxy.go` | Modify | Model extraction from request body, `ctxKeyModel`, updated `ModifyResponse` |
| `inspect.go` | Modify | Extended `ollamaUsage`, model-aware `inspectingReader`, `RecordResponse` call |
| `main_test.go` | Modify | Updated existing tests + five new tests |

## Data Models

### `UsageStat` (new)

```go
type UsageStat struct {
    RequestCount         int64  `json:"request_count"`
    ResponseCount        int64  `json:"response_count"`
    LastRequestTimestamp string `json:"last_request_timestamp"`
    TotalDuration        int64  `json:"total_duration"`
    LoadDuration         int64  `json:"load_duration"`
    PromptEvalCount      int64  `json:"prompt_eval_count"`
    PromptEvalDuration   int64  `json:"prompt_eval_duration"`
    EvalCount            int64  `json:"eval_count"`
    EvalDuration         int64  `json:"eval_duration"`
}
```

### `UsageStore.data` (new type)

```go
data map[string]map[string]map[string]*UsageStat // [isodate][apikey][model]
```

### `ollamaUsage` (extended)

```go
type ollamaUsage struct {
    Done               *bool  `json:"done"`
    TotalDuration      int64  `json:"total_duration"`
    LoadDuration       int64  `json:"load_duration"`
    PromptEvalCount    int64  `json:"prompt_eval_count"`
    PromptEvalDuration int64  `json:"prompt_eval_duration"`
    EvalCount          int64  `json:"eval_count"`
    EvalDuration       int64  `json:"eval_duration"`
}
```

## API Contracts

`GET /usage` response shape (unchanged endpoint, new body structure):

```json
{
  "usage": {
    "2026-04-17": {
      "my-api-key": {
        "qwen3.5:0.8b": {
          "request_count": 3,
          "response_count": 3,
          "last_request_timestamp": "2026-04-17T01:02:14Z",
          "total_duration": 41076672310,
          "load_duration": 10153504796,
          "prompt_eval_count": 54,
          "prompt_eval_duration": 124025500,
          "eval_count": 4155,
          "eval_duration": 30171368949
        }
      }
    }
  }
}
```

## Key Code Snippets

### `store.go` — new method signatures

```go
// RecordRequest increments request_count and updates last_request_timestamp.
func (s *UsageStore) RecordRequest(date, token, model, timestamp string) { ... }

// RecordResponse increments response_count and accumulates timing/token fields.
func (s *UsageStore) RecordResponse(date, token, model string, u ollamaUsage) { ... }

// Snapshot returns a deep copy keyed by [date][token][model].
func (s *UsageStore) Snapshot() map[string]map[string]map[string]UsageStat { ... }

// getOrCreate — now takes three keys.
func (s *UsageStore) getOrCreate(date, token, model string) *UsageStat { ... }
```

### `proxy.go` — model extraction + context key

```go
type proxyContextKey int
const ctxKeyModel proxyContextKey = iota

// extractModel reads the request body JSON, pulls out "model", then restores the body.
func extractModel(req *http.Request) string {
    if req.Body == nil {
        return "unknown"
    }
    data, err := io.ReadAll(req.Body)
    req.Body.Close()
    req.Body = io.NopCloser(bytes.NewReader(data))
    if err != nil {
        return "unknown"
    }
    var v struct{ Model string `json:"model"` }
    if json.Unmarshal(data, &v) != nil || v.Model == "" {
        return "unknown"
    }
    return v.Model
}
```

Director:
```go
proxy.Director = func(req *http.Request) {
    model := extractModel(req)
    *req = *req.WithContext(context.WithValue(req.Context(), ctxKeyModel, model))
    baseDirector(req)
    req.Header.Del("Authorization")
}
```

ModifyResponse:
```go
proxy.ModifyResponse = func(resp *http.Response) error {
    token, _ := resp.Request.Context().Value(ctxKeyToken).(string)
    model, _ := resp.Request.Context().Value(ctxKeyModel).(string)
    date := time.Now().UTC().Format("2006-01-02")
    ts   := time.Now().UTC().Format(time.RFC3339)

    store.RecordRequest(date, token, model, ts)
    resp.Body = newInspectingReader(resp.Body, store, token, model)
    return nil
}
```

### `inspect.go` — updated constructor and tryRecord

```go
func newInspectingReader(body io.ReadCloser, store *UsageStore, token, model string) *inspectingReader {
    return &inspectingReader{
        body:  body,
        store: store,
        date:  time.Now().UTC().Format("2006-01-02"),
        token: token,
        model: model,
    }
}

func (r *inspectingReader) tryRecord(data []byte) {
    var u ollamaUsage
    if err := json.Unmarshal(data, &u); err != nil {
        return
    }
    shouldRecord := false
    if u.Done != nil {
        shouldRecord = *u.Done
    } else if u.PromptEvalCount > 0 || u.EvalCount > 0 {
        shouldRecord = true
    }
    if shouldRecord {
        r.store.RecordResponse(r.date, r.token, r.model, u)
    }
}
```

## Unit Tests

| Test | Input | Expected Output |
|------|-------|-----------------|
| `TestStore_MultiModelBucketing` | Same token, two different models, one request each | Snapshot has two separate model entries under the same token |
| `TestStore_ResponseCountVsRequestCount` | RecordRequest called 3×, RecordResponse called 2× | `request_count=3`, `response_count=2` |
| `TestStore_LastRequestTimestampUpdated` | RecordRequest called twice with different timestamps | `last_request_timestamp` equals the second (later) timestamp |
| `TestStore_TimingFieldsAccumulated` | RecordResponse called twice with known timing values | All timing fields equal the sum of both calls |
| `TestInspectingReader_ModelRouting` | Two readers with different models, one request each | Each model bucket has `response_count=1`, `eval_count` correct |
| `TestProxyIntegration_ModelExtracted` | POST `/api/generate` with body `{"model":"test-model"}` | Snapshot contains "test-model" bucket under the API key |

> Existing tests `TestStore_RecordRequestAndUsage`, `TestStore_MultipleTokensAndDates`, all `TestInspectingReader_*`, `TestProxyIntegration_*`, `TestUsageEndpoint_ReturnsJSON`, and `TestRequestCounter_*` must be updated to use the new method signatures, field names, and three-level snapshot structure.

## Risks & Open Questions

- **Request body size**: `extractModel` reads the full body into memory before restoring it. For typical Ollama `/api/generate` bodies this is fine (small JSON). No mitigation needed for this use case.
- **Body read on non-JSON requests**: `extractModel` falls back to `"unknown"` gracefully on any parse failure.
- **`last_request_timestamp` precision**: Using `time.RFC3339` (second-level precision) which is sufficient for a usage dashboard. Nanosecond precision is not needed.
