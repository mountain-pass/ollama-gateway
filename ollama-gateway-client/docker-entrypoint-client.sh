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
