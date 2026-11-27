#!/usr/bin/env bash
set -Eeuo pipefail
if (( $# )); then exec "$@"; fi
if [[ -z "${MMDB_PATH:-}" ]]; then
 if [[ -e /data/country.mmdb ]]; then
  export MMDB_PATH=/data/country.mmdb
 else
  export MMDB_PATH=/app/geo/country.mmdb
 fi
fi
# Only validated decimal values are substituted; Nginx variables stay literal.
DSP_RPS="${DSP_RPS:-3000}"
DSP_BURST="${DSP_BURST:-100}"
for setting in DSP_RPS DSP_BURST; do
 value="${!setting}"
 [[ "$value" =~ ^[1-9][0-9]{0,6}$ ]] || { echo "$setting must be an integer from 1 to 9999999" >&2; exit 1; }
done
config="$(cat /app/nginx.conf.template)"
config="${config//__DSP_RPS__/$DSP_RPS}"
config="${config//__DSP_BURST__/$DSP_BURST}"
printf '%s\n' "$config" > /tmp/bidhelm-nginx.conf
mkdir -p /tmp/bidhelm-nginx/body /tmp/bidhelm-nginx/proxy
nginx -t -c /tmp/bidhelm-nginx.conf
# The container's application listener is always private.
export LISTEN_ADDR=127.0.0.1:8083
/app/backoffice &
bo=$!
/app/dsp &
dsp=$!
nginx -c /tmp/bidhelm-nginx.conf -g 'daemon off;' &
nginx_pid=$!
stop() {
 kill -QUIT "$nginx_pid" 2>/dev/null || true
 kill -TERM "$bo" "$dsp" 2>/dev/null || true
 wait "$nginx_pid" 2>/dev/null || true
 wait "$bo" 2>/dev/null || true
 wait "$dsp" 2>/dev/null || true
}
trap 'stop; exit 0' TERM INT
set +e
wait -n "$bo" "$dsp" "$nginx_pid"
code=$?
set -e
stop
# An unexpected clean child exit is also a container failure.
if (( code == 0 )); then code=1; fi
exit "$code"
