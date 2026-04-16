# Gateway SSL (Frontend) — Implementation Plan

**Requirement**: [2026-04-16-gateway-ssl-frontend.md](2026-04-16-gateway-ssl-frontend.md)
**Date**: 2026-04-16
**Status**: Approved

## Implementation Steps

1. **`main.go` — read and validate `HTTPS` env var**
   Read `HTTPS`; normalise to lowercase. If not `""`, `"true"`, or `"false"`, call `log.Fatal` with a descriptive message. Derive `httpsEnabled bool`.

2. **`main.go` — read `HTTPS_CERTIFICATE` and `HTTPS_PRIVATE_KEY` with defaults**
   If `HTTPS_CERTIFICATE` is unset, default to `/app/cert.pem`. Same for `HTTPS_PRIVATE_KEY` → `/app/key.pem`.

3. **`main.go` — validate file existence when `HTTPS=true`**
   When `httpsEnabled`, call `os.Stat` on each path. If either returns an error, call `log.Fatalf` before binding.

4. **`main.go` — conditional listener**
   Replace the single `http.ListenAndServe` call with an `if/else`:
   - `httpsEnabled` → `http.ListenAndServeTLS(addr, certFile, keyFile, mux)`
   - otherwise → `http.ListenAndServe(addr, mux)`
   Log `https` or `http` in the startup message accordingly.

5. **`README.md` — document new env vars**
   Add `HTTPS`, `HTTPS_CERTIFICATE`, and `HTTPS_PRIVATE_KEY` to the environment variables table with their defaults and a brief note about the file-existence check.

## File Change Summary

| File | Action | Description |
|------|--------|-------------|
| `main.go` | Modify | Add HTTPS env var parsing, file existence checks, conditional TLS listener |
| `README.md` | Modify | Document three new env vars in the configuration table |

## Key Code Snippets

```go
// Env var parsing (after PORT block)
httpsRaw := strings.ToLower(os.Getenv("HTTPS"))
if httpsRaw != "" && httpsRaw != "true" && httpsRaw != "false" {
    log.Fatalf("HTTPS must be \"true\" or \"false\", got %q", httpsRaw)
}
httpsEnabled := httpsRaw == "true"

certFile := os.Getenv("HTTPS_CERTIFICATE")
if certFile == "" {
    certFile = "/app/cert.pem"
}
keyFile := os.Getenv("HTTPS_PRIVATE_KEY")
if keyFile == "" {
    keyFile = "/app/key.pem"
}

if httpsEnabled {
    if _, err := os.Stat(certFile); err != nil {
        log.Fatalf("HTTPS_CERTIFICATE file not found: %v", err)
    }
    if _, err := os.Stat(keyFile); err != nil {
        log.Fatalf("HTTPS_PRIVATE_KEY file not found: %v", err)
    }
}

// Listener
addr := ":" + port
if httpsEnabled {
    log.Printf("ollama-gateway listening on https://%s, proxying to %s", addr, ollamaBaseURL)
    if err := http.ListenAndServeTLS(addr, certFile, keyFile, mux); err != nil {
        log.Fatalf("Server error: %v", err)
    }
} else {
    log.Printf("ollama-gateway listening on http://%s, proxying to %s", addr, ollamaBaseURL)
    if err := http.ListenAndServe(addr, mux); err != nil {
        log.Fatalf("Server error: %v", err)
    }
}
```

## Unit Tests

| Test | Input | Expected Output |
|------|-------|-----------------|
| Invalid HTTPS value | `HTTPS=yes` | `log.Fatal` before bind |
| HTTPS=false (default) | no HTTPS env var | plain HTTP listener starts |
| HTTPS=true, files missing | cert/key paths don't exist | `log.Fatal` with file path in message |
| HTTPS=true, files present | valid cert+key | `ListenAndServeTLS` called (integration) |

> Note: `log.Fatal` calls in `main()` make unit testing the startup path tricky in Go. The file-existence and value-validation logic can be extracted into small helper functions for testability if desired, but that is out of scope for this requirement.

## Risks & Open Questions

- None. Change is isolated to startup logic in `main.go` and documentation in `README.md`.
