#!/usr/bin/env bash
# Run inside the DSP container: docker exec bidhelm /app/metrics.sh
set -Eeuo pipefail
token_file="${TOKEN_FILE:-/data/internal-token}"
IFS= read -r token < "$token_file" || [[ -n "${token:-}" ]]
[[ -n "$token" && "$token" != *'"'* && "$token" != *'\'* ]] || { echo 'Invalid internal token file' >&2; exit 1; }
# Keep the token out of process arguments and terminal output.
printf 'header = "Authorization: Bearer %s"\n' "$token" |
 curl --fail --silent --show-error --max-time 5 --config - http://127.0.0.1:8082/internal/metrics
