# Usage Endpoint Dynamic Filtering

**Date Added**: 2026-04-20
**Priority**: High
**Status**: Completed

## Problem Statement

The `/usage` endpoint currently returns the full three-level snapshot (`date → user → model → stats`) in a single response. There is no way to drill into a specific date, user, or model without fetching and filtering the entire payload client-side.

## Functional Requirements

1. `GET /usage` — returns the full snapshot, keyed by date → user → model → stats (same data as now, but **without** the `{"usage": ...}` wrapper — direct JSON object).
2. `GET /usage/{date}` — returns only the data for that ISO date: `{ "<user>": { "<model>": { ...stats } } }`.
3. `GET /usage/{date}/{user}` — returns only the data for that date + user: `{ "<model>": { ...stats } }`.
4. `GET /usage/{date}/{user}/{model}` — returns only the stats for that date + user + model: `{ ...stats }`.
5. If a requested date / user / model key does not exist in the store, return HTTP 404 with a JSON error body.

## User Experience Requirements

- All four URL shapes are handled by a single handler registered under `/usage/` (trailing slash to capture sub-paths).
- The `{"usage": ...}` envelope is removed; each endpoint returns data directly at the appropriate nesting level.
- Response `Content-Type` is `application/json` for all variants.

## Technical Requirements

- Use Go 1.22+ `http.ServeMux` path-parameter routing (e.g. `/usage/{date}/{user}/{model}`) OR strip the prefix manually and parse path segments — the simpler approach given the existing Go version.
- The `UsageStore` needs snapshot methods scoped to each level, or the existing `Snapshot()` is used and the handler walks the result to the requested depth.
- Thread-safety is unchanged — snapshots are already safe copies.
- The model path segment may contain `/` characters (e.g. `qwen3.5:0.8b` does not, but future model tags might use `/`). For now, treat the third segment as a single path component (URL-encoded slashes become `%2F`).

## Acceptance Criteria

- [x] `GET /usage` returns the full date→user→model→stats map directly (no `"usage"` wrapper).
- [x] `GET /usage/2026-04-20` returns the user→model→stats map for that date, or 404 if not found.
- [x] `GET /usage/2026-04-20/nick` returns the model→stats map for that date+user, or 404 if not found.
- [x] `GET /usage/2026-04-20/nick/llama3` returns the stats object for that date+user+model, or 404 if not found.
- [x] All existing tests continue to pass.
- [x] New tests cover each URL variant and the 404 paths.

## Dependencies

- REQ-003 (Track API Usage Metrics by Date / API Key / Model) — this requirement extends the `/usage` endpoint introduced there.
- REQ-004 (Usage Tracking Fixes) — must remain compatible.

## Implementation Notes

- Register the handler under `/usage` and `/usage/` in the mux, or use a single `/usage` prefix handler that strips the prefix.
- Parse path segments after stripping `/usage` prefix: split by `/`, skip empty strings, and branch on segment count (0, 1, 2, 3).
- Return `{"error": "not found"}` with HTTP 404 for missing keys.
- The `"usage"` wrapper removal is a breaking change to existing clients of `GET /usage` — flag this clearly.
