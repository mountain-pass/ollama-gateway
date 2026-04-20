# Usage Tracking Fixes: Exclude /usage Path, Restore total_tokens

**Date Added**: 2026-04-20
**Priority**: High
**Status**: Completed

## Problem Statement

Three corrections to the usage tracking introduced in REQ-003:

1. `request_count` is currently incremented inside `ModifyResponse`, which fires only after Ollama returns a response. If Ollama times out or errors, the request goes uncounted. It must be incremented in `Director`, which fires as the request is forwarded — before any response is received.
2. The `/usage` endpoint should never contribute to `request_count` or `response_count`. Because `Director` only runs for proxied requests, `/usage` (mux-routed to `usageHandler`) will never reach it. An explicit path guard in `ModifyResponse` adds defence-in-depth for the response-wrapping step.
3. `total_tokens` (`prompt_eval_count + eval_count`) was removed in REQ-003 but should be retained for convenience.

## Functional Requirements

1. `request_count` is incremented when the request is forwarded to Ollama (in `Director`), not when the response arrives.
2. The proxy must not record request or response metrics for any request whose path is `/usage`.
3. `UsageStat` must include a `total_tokens` field equal to the cumulative sum of `prompt_eval_count + eval_count`.

## Technical Requirements

- Move `store.RecordRequest(...)` from `ModifyResponse` into `Director`. The token (`ctxKeyToken`) and model (`ctxKeyModel`) are both already in context at that point.
- Remove the `RecordRequest` call from `ModifyResponse`; keep only the body-wrapping step there, guarded by a path check (`!= "/usage"`).
- `total_tokens` is accumulated in `store.go` `RecordResponse` as `prompt_eval_count + eval_count`.
- No changes to `inspect.go`, `usage.go`, or `auth.go`.

## Acceptance Criteria

- [ ] `request_count` is incremented even when Ollama returns an error (i.e. before response arrives).
- [ ] A request to `/usage` does not increment `request_count` in the store.
- [ ] `UsageStat.TotalTokens` equals `PromptEvalCount + EvalCount` after a `RecordResponse` call.
- [ ] All existing tests continue to pass.
- [ ] New test: `request_count` is recorded even when backend returns a non-200 / error response.
- [ ] New test: proxy integration confirms `/usage` request leaves store empty.
- [ ] New test: `total_tokens` is correct after one and two `RecordResponse` calls.

## Dependencies

- REQ-003 (Track API Usage Metrics by Date / API Key / Model) — this requirement amends that work.
