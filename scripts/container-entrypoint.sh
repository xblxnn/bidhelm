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
/app/backoffice &
bo=$!
/app/dsp &
dsp=$!
stop() { kill -TERM "$bo" "$dsp" 2>/dev/null || true; wait "$bo" 2>/dev/null || true; wait "$dsp" 2>/dev/null || true; }
trap 'stop; exit 0' TERM INT
set +e
wait -n "$bo" "$dsp"
code=$?
set -e
stop
# An unexpected clean child exit is also a container failure.
if (( code == 0 )); then code=1; fi
exit "$code"
