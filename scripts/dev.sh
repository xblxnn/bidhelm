#!/usr/bin/env bash
# Local-only launch. No Docker daemon or remote deployment is needed.
set -Eeuo pipefail
cd "$(dirname "$0")/.."
umask 077
mkdir -p data bin
export DB_PATH="$PWD/data/app.db" TOKEN_FILE="$PWD/data/internal-token" MMDB_PATH="${MMDB_PATH:-}"
if [[ -z "$MMDB_PATH" ]]; then
 if [[ -e "$PWD/data/country.mmdb" ]]; then export MMDB_PATH="$PWD/data/country.mmdb";
 else export MMDB_PATH="$PWD/assets/geo/dbip-country-lite.mmdb"; fi
fi
export PUBLIC_URL=http://127.0.0.1:8080 LISTEN_ADDR=127.0.0.1:8080
[[ -f "$TOKEN_FILE" ]] || openssl rand -hex 32 > "$TOKEN_FILE"
go build -o bin/backoffice ./cmd/backoffice
go build -o bin/dsp ./cmd/dsp
if ! bin/backoffice has-admin; then
 login="${ADMIN_LOGIN:-admin}"
 password="$(openssl rand -hex 16)"
 printf '%s\n' "$password" | bin/backoffice init "$login"
 printf 'URL: %s\nLogin: %s\nPassword: %s\n' "$PUBLIC_URL" "$login" "$password" > data/local-access.txt
 chmod 600 data/local-access.txt
 cat data/local-access.txt
 unset password
fi
bin/backoffice & bo=$!
bin/dsp & dsp=$!
stop() { kill -TERM "$bo" "$dsp" 2>/dev/null || true; wait "$bo" 2>/dev/null || true; wait "$dsp" 2>/dev/null || true; }
trap stop EXIT
trap 'exit 0' INT TERM HUP
printf 'Local backoffice: %s\n' "$PUBLIC_URL"
# Bash 3 on macOS does not support wait -n.
while kill -0 "$bo" 2>/dev/null && kill -0 "$dsp" 2>/dev/null; do sleep 1; done
stop
exit 1
