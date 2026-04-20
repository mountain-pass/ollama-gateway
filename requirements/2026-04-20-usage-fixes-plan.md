# Usage Tracking Fixes — Implementation Plan

**Requirement**: [2026-04-20-usage-fixes.md](2026-04-20-usage-fixes.md)
**Date**: 2026-04-20
**Status**: Implemented

## Implementation Steps

1. **`store.go`** — Add `TotalTokens int64` field to `UsageStat`; accumulate it in `RecordResponse` as `prompt_eval_count + eval_count`.

2. **`proxy.go`** — In `Director`, after extracting model and injecting it into context, call `store.RecordRequest(date, token, model, ts)`. Remove the `RecordRequest` call from `ModifyResponse`. Add a path guard in `ModifyResponse`: if `resp.Request.URL.Path == "/usage"`, return nil immediately without wrapping the body.

3. **`main_test.go`** — Update `TestProxyIntegration_NonStreamingUsageCaptured` and `TestRequestCounter_IncrementedPerRequest` to match the new call site (no functional change to those tests). Add three new tests: `TestStore_TotalTokensAccumulated`, `TestProxyIntegration_RequestCountOnBackendError`, `TestProxyIntegration_UsagePathNotCounted`.

## File Change Summary

| File | Action | Description |
|------|--------|-------------|
| `store.go` | Modify | Add `TotalTokens`; accumulate in `RecordResponse` |
| `proxy.go` | Modify | Move `RecordRequest` to `Director`; add `/usage` guard in `ModifyResponse` |
| `main_test.go` | Modify | Three new tests; minor updates to existing proxy tests |

## Key Code Snippets

### `store.go` — updated `UsageStat` and `RecordResponse`

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
    TotalTokens          int64  `json:"total_tokens"`
}

func (s *UsageStore) RecordResponse(date, token, model string, u ollamaUsage) {
    s.mu.Lock()
    defer s.mu.Unlock()
    st := s.getOrCreate(date, token, model)
    st.ResponseCount++
    st.TotalDuration += u.TotalDuration
    st.LoadDuration += u.LoadDuration
    st.PromptEvalCount += u.PromptEvalCount
    st.PromptEvalDuration += u.PromptEvalDuration
    st.EvalCount += u.EvalCount
    st.EvalDuration += u.EvalDuration
    st.TotalTokens += u.PromptEvalCount + u.EvalCount
}
```

### `proxy.go` — `Director` records request; `ModifyResponse` guarded

```go
proxy.Director = func(req *http.Request) {
    model := extractModel(req)
    *req = *req.WithContext(context.WithValue(req.Context(), ctxKeyModel, model))
    baseDirector(req)
    req.Header.Del("Authorization")

    // Count the request here so it's recorded even if Ollama never responds.
    token, _ := req.Context().Value(ctxKeyToken).(string)
    now := time.Now().UTC()
    store.RecordRequest(now.Format("2006-01-02"), token, model, now.Format(time.RFC3339))
}

proxy.ModifyResponse = func(resp *http.Response) error {
    if resp.Request.URL.Path == "/usage" {
        return nil
    }
    token, _ := resp.Request.Context().Value(ctxKeyToken).(string)
    model, _ := resp.Request.Context().Value(ctxKeyModel).(string)
    resp.Body = newInspectingReader(resp.Body, store, token, model)
    return nil
}
```

## Unit Tests

| Test | Input | Expected Output |
|------|-------|-----------------|
| `TestStore_TotalTokensAccumulated` | Two `RecordResponse` calls with `prompt_eval_count=10,eval_count=5` each | `total_tokens == 30` |
| `TestProxyIntegration_RequestCountOnBackendError` | Backend closes connection immediately (500 or drops) | `request_count >= 1` in snapshot despite error response |
| `TestProxyIntegration_UsagePathNotCounted` | Direct call to `/usage` endpoint | Snapshot for the token has no entries (or all zero counts) |

## Risks & Open Questions

- `Director` runs synchronously before forwarding. Calling `store.RecordRequest` here is safe — the mutex is quick and non-blocking.
- After moving `RecordRequest` to `Director`, `ModifyResponse` no longer needs `date`/`ts` locals; they are removed to keep the function clean.
