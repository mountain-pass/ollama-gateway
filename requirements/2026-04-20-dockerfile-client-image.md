# Dockerfile Client Image

**Date Added**: 2026-04-20
**Priority**: Medium
**Status**: Completed

## Problem Statement

The `docker-compose.client.yml` file provides a useful nginx-based local proxy that forwards requests to the ollama-gateway server with a hardcoded API key. The target URL and API key are baked into the compose file, making it unsuitable for distribution as a reusable image. A standalone Docker image is needed that can be configured at runtime via environment variables.

## Functional Requirements

1. A `Dockerfile.client` must be created that builds an nginx-based client proxy image.
2. The image must accept two environment variables at runtime:
   - `TARGET` — the URL of the target ollama-gateway server (e.g. `https://202.137.172.189:11434`)
   - `APIKEY` — the Bearer token to use when authenticating to the gateway
3. On container startup, the entrypoint script must:
   - Check that `TARGET` is set and non-empty; if not, print a clear error message and exit with a non-zero code.
   - Check that `APIKEY` is set and non-empty; if not, print a clear error message and exit with a non-zero code.
   - Generate `/etc/nginx/nginx.conf` dynamically using the values of `TARGET` and `APIKEY`.
   - Start nginx in the foreground (`nginx -g 'daemon off;'`).
4. The nginx configuration must:
   - Listen on port 11434.
   - Proxy all requests to the `TARGET` URL.
   - Set `Authorization: Bearer <APIKEY>` on every proxied request.
   - Set the `Host` header to the hostname of `TARGET`.
   - Disable SSL verification (`proxy_ssl_verify off`) and enable SNI (`proxy_ssl_server_name on`) for HTTPS targets.
   - Use HTTP/1.1 and clear the `Connection` header for keep-alive compatibility.

## User Experience Requirements

- The published image name is `mountainpass/ollama-gateway-client`.
- Usage is as simple as:
  ```
  docker run -d -e TARGET=https://202.137.172.189:11434 -e APIKEY=nick -p 11434:11434 mountainpass/ollama-gateway-client
  ```
- Missing environment variables must produce a human-readable fatal error, not a cryptic nginx failure.

## Technical Requirements

- Base image: `nginx:alpine` (smaller footprint than `nginx:latest`).
- Entrypoint: a shell script (`/docker-entrypoint-client.sh`) that validates env vars, writes nginx.conf, then `exec`s nginx.
- The `HOST` header must be derived from the hostname portion of `TARGET` (strip protocol and port).

## Acceptance Criteria

- [ ] `Dockerfile.client` exists and builds successfully.
- [ ] Container exits with a non-zero code and a clear error message if `TARGET` is unset.
- [ ] Container exits with a non-zero code and a clear error message if `APIKEY` is unset.
- [ ] Container starts and nginx listens on port 11434 when both env vars are provided.
- [ ] Requests to the container are proxied to `TARGET` with the correct `Authorization` header.
- [ ] README documents the image, its environment variables, and a usage example.

## Dependencies

- Existing `docker-compose.client.yml` (reference for nginx config structure).
- README.md (to be updated with usage docs).

## Implementation Notes

- The host extraction from `TARGET` can be done with shell parameter expansion or `sed`.
- Use `exec nginx` (not a subshell) so nginx receives signals correctly.
