# Track API Usage Metrics by Date / API Key / Model

**Date Added**: 2026-04-17
**Priority**: High
**Status**: Completed

## Problem Statement

The current usage store tracks metrics keyed by `[isodate][token]` and only captures request counts and token counts (`prompt_tokens`, `completion_tokens`, `total_tokens`). This does not distinguish between different models used by the same API key, and it is missing several useful timing and throughput fields that Ollama already returns in every response.

## Functional Requirements

1. Change the storage key from `[isodate][token]` to `[isodate][apikey][model]`.
2. Capture the model name from the request payload (the `model` field in the JSON body sent by the client to Ollama).
3. Track the following fields per `(date, apikey, model)` bucket:
   - `request_count` — incremented each time a request is forwarded (rename from `requests`)
   - `response_count` — incremented each time a complete Ollama response is received (i.e. `"done": true`)
   - `last_request_timestamp` — RFC-3339 / ISO-8601 timestamp of the most recent request
   - `total_duration` — cumulative sum of `total_duration` from Ollama responses (nanoseconds)
   - `load_duration` — cumulative sum of `load_duration` from Ollama responses (nanoseconds)
   - `prompt_eval_count` — cumulative sum of `prompt_eval_count` (rename from `prompt_tokens`)
   - `prompt_eval_duration` — cumulative sum of `prompt_eval_duration` from Ollama responses (nanoseconds)
   - `eval_count` — cumulative sum of `eval_count` (rename from `completion_tokens`)
   - `eval_duration` — cumulative sum of `eval_duration` from Ollama responses (nanoseconds)
4. The existing `total_tokens` field is removed (it was `prompt_tokens + completion_tokens`, derivable from `prompt_eval_count + eval_count`).
5. The `/usage` endpoint continues to return a JSON snapshot in the new nested structure.

## User Experience Requirements

- No change to how clients authenticate or make requests.
- The `/usage` endpoint response shape changes to `{ "usage": { "<date>": { "<apikey>": { "<model>": { ... } } } } }`.

## Technical Requirements

- Model name must be read from the proxied **request** body before it is forwarded to Ollama. The request body must remain readable by the downstream proxy (i.e. it must be buffered/reset after reading).
- Thread-safety: all store mutations must remain behind the existing mutex.
- The `inspectingReader` must be extended to parse the additional Ollama response fields (`total_duration`, `load_duration`, `prompt_eval_duration`, `eval_duration`).
- `last_request_timestamp` is set at request time (in `ModifyResponse`), not at response-read time.
- Existing tests must be updated to reflect the new data model; new tests must cover model-level bucketing and the new fields.

## Acceptance Criteria

- [ ] Usage is stored as `map[isodate][apikey][model] => UsageStat`.
- [ ] `request_count` is incremented for every forwarded request.
- [ ] `response_count` is incremented only when `done: true` is seen in the response.
- [ ] `last_request_timestamp` reflects the wall-clock time of the most recent request to that bucket.
- [ ] `total_duration`, `load_duration`, `prompt_eval_count`, `prompt_eval_duration`, `eval_count`, `eval_duration` are accumulated correctly from Ollama response payloads.
- [ ] The `/usage` JSON output uses the new three-level key structure.
- [ ] All existing tests pass (updated to new field names / structure).
- [ ] New tests cover: multi-model bucketing, response_count vs request_count divergence, timestamp update, and all new numeric fields.

## Dependencies

- REQ-001 (Ollama HTTP Proxy Gateway) — this requirement modifies the store and inspecting-reader introduced there.

## Implementation Notes

- The model name should be extracted from the request body JSON (`"model"` field). If the field is absent or the body is not JSON, use the sentinel value `"unknown"`.
- `last_request_timestamp` should be stored as a `string` (RFC-3339) to keep marshalling simple.
- Timing fields are `int64` nanoseconds, matching Ollama's native units.
