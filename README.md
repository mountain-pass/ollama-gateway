# ollama-gateway

A lightweight authenticated HTTP proxy that sits in front of a single [Ollama](https://ollama.com) instance. It enforces API token authentication, tracks per-token token usage in memory, and exposes a `/usage` endpoint.

## Architecture

```
Client
  │
  │  Authorization: Bearer <token>
  ▼
┌─────────────────────────────┐
│       ollama-gateway        │
│                             │
│  ┌─────────────────────┐    │
│  │  Auth middleware    │    │
│  │  (Bearer token)     │    │
│  └────────┬────────────┘    │
│           │                 │
│    ┌──────┴──────┐          │
│    │             │          │
│  /usage      all other      │
│  handler     requests       │
│    │             │          │
│    │         ┌───┴──────┐   │
│    │         │  Reverse │   │
│    │         │  Proxy   │   │
│    │         └───┬──────┘   │
│    │             │          │
│  Usage      Inspect body    │
│  Store  ◄── for token usage │
│             │               │
└─────────────┼───────────────┘
              │  plain HTTP
              ▼
        Ollama instance
        (OLLAMA_BASE_URL)
```

## Prerequisites

- Go 1.21 or later
- A running Ollama instance

## Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `OLLAMA_BASE_URL` | Yes | — | Base URL of the Ollama backend (e.g. `http://localhost:11434`) |
| `API_TOKENS` | Yes | — | Comma-separated list of valid Bearer tokens (e.g. `token-a,token-b`) |
| `PORT` | No | `8080` | Port the gateway listens on |

## Quick Start

**Build:**
```bash
make build
# or
CGO_ENABLED=0 go build -o ollama-gateway .
```

**Run:**
```bash
export OLLAMA_BASE_URL=http://localhost:11434
export API_TOKENS=my-secret-token,another-token
export PORT=8080          # optional

./ollama-gateway
# ollama-gateway listening on :8080, proxying to http://localhost:11434
```

**Use it exactly like the Ollama API** — just add the `Authorization` header:
```bash
# List models
curl http://localhost:8080/api/tags \
  -H "Authorization: Bearer my-secret-token"

# Generate (non-streaming)
curl http://localhost:8080/api/generate \
  -H "Authorization: Bearer my-secret-token" \
  -d '{"model":"llama3","prompt":"Why is the sky blue?","stream":false}'

# Generate (streaming)
curl http://localhost:8080/api/generate \
  -H "Authorization: Bearer my-secret-token" \
  -d '{"model":"llama3","prompt":"Why is the sky blue?"}'
```

**Without a valid token — 401:**
```bash
curl -i http://localhost:8080/api/tags
# HTTP/1.1 401 Unauthorized
```

## Usage Endpoint

`GET /usage` returns a JSON snapshot of accumulated token usage, grouped by date then token. The same authentication requirement applies.

```bash
curl http://localhost:8080/usage \
  -H "Authorization: Bearer my-secret-token"
```

```json
{
  "usage": {
    "2026-04-16": {
      "my-secret-token": {
        "requests": 5,
        "prompt_tokens": 120,
        "completion_tokens": 340,
        "total_tokens": 460
      },
      "another-token": {
        "requests": 2,
        "prompt_tokens": 40,
        "completion_tokens": 95,
        "total_tokens": 135
      }
    }
  }
}
```

> Usage is tracked in memory and resets when the gateway restarts.

## How Token Usage is Captured

Ollama responses include token counts in JSON fields `prompt_eval_count` and `eval_count`. The gateway inspects every response body without buffering:

- **Streaming responses** (NDJSON): each chunk is flushed to the client immediately; the final `"done": true` object's counts are recorded.
- **Non-streaming responses**: the single JSON object is inspected on close.

The request counter is incremented unconditionally for every proxied request, regardless of whether the response contains usage fields.

## Development

```bash
make test    # run tests with race detector
make build   # produce ./ollama-gateway binary
make run     # build and run with example env vars
```
