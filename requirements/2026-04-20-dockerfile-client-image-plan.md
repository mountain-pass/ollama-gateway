# Dockerfile Client Image — Implementation Plan

**Requirement**: [2026-04-20-dockerfile-client-image.md](2026-04-20-dockerfile-client-image.md)
**Date**: 2026-04-20
**Status**: Implemented

## Implementation Steps

1. **Create `docker-entrypoint-client.sh`** — shell script that:
   - Validates `TARGET` is set and non-empty; prints `ERROR: TARGET environment variable is required` and exits 1 if not.
   - Validates `APIKEY` is set and non-empty; prints `ERROR: APIKEY environment variable is required` and exits 1 if not.
   - Extracts the hostname from `TARGET` using `sed` (strips `https://` / `http://` and any trailing path/port for the Host header, but keeps the raw host:port for proxy_pass).
   - Writes `/etc/nginx/nginx.conf` using a heredoc with `$TARGET`, `$APIKEY`, and the extracted hostname interpolated.
   - `exec nginx -g 'daemon off;'`

2. **Create `Dockerfile.client`** — Docker image definition that:
   - Uses `FROM nginx:alpine`
   - Copies `docker-entrypoint-client.sh` into the image at `/docker-entrypoint-client.sh`
   - Makes it executable (`RUN chmod +x /docker-entrypoint-client.sh`)
   - Exposes port 11434
   - Sets `ENTRYPOINT ["/docker-entrypoint-client.sh"]`

3. **Update `README.md`** — add a new "Client Image" section after the existing content documenting:
   - What the image does
   - The two environment variables (`TARGET`, `APIKEY`)
   - The `docker run` usage example
   - A note that the image is available as `mountainpass/ollama-gateway-client`

## File Change Summary

| File | Action | Description |
|------|--------|-------------|
| `docker-entrypoint-client.sh` | Create | Entrypoint: validates env vars, writes nginx.conf, starts nginx |
| `Dockerfile.client` | Create | Builds `mountainpass/ollama-gateway-client` from `nginx:alpine` |
| `README.md` | Modify | Add "Client Image" section with env vars table and usage example |
| `requirements/2026-04-20-dockerfile-client-image.md` | Modify | Status → In Progress / Completed |
| `requirements/_index.md` | Modify | Status → In Progress / Completed |

## Key Code Snippets

### `docker-entrypoint-client.sh`

```sh
#!/bin/sh
set -e

if [ -z "$TARGET" ]; then
  echo "ERROR: TARGET environment variable is required (e.g. https://192.168.1.1:11434)"
  exit 1
fi

if [ -z "$APIKEY" ]; then
  echo "ERROR: APIKEY environment variable is required (e.g. my-secret-token)"
  exit 1
fi

# Extract bare hostname (no protocol, port, or path) for the Host header
TARGET_HOST=$(echo "$TARGET" | sed 's|^https\?://||' | sed 's|/.*||' | sed 's|:.*||')

cat > /etc/nginx/nginx.conf <<EOF
events {}

http {
    server {
        listen 11434;

        location / {
            proxy_pass $TARGET;

            proxy_ssl_server_name on;
            proxy_ssl_verify off;

            proxy_set_header Host $TARGET_HOST;
            proxy_set_header Authorization "Bearer $APIKEY";

            proxy_http_version 1.1;
            proxy_set_header Connection "";
        }
    }
}
EOF

exec nginx -g 'daemon off;'
```

### `Dockerfile.client`

```dockerfile
FROM nginx:alpine

COPY docker-entrypoint-client.sh /docker-entrypoint-client.sh
RUN chmod +x /docker-entrypoint-client.sh

EXPOSE 11434

ENTRYPOINT ["/docker-entrypoint-client.sh"]
```

## Risks & Open Questions

- The `Host` header value: matches the existing `docker-compose.client.yml` — bare hostname/IP only (no port). Protocol, port, and path are all stripped. The port lives in the `proxy_pass` URL, not the header.
- `proxy_ssl_verify off` is intentional to match existing behaviour (self-signed certs on the gateway).
- No `.dockerignore` changes needed — the entrypoint script is small and safe to copy.
