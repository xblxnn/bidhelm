#!/usr/bin/env bash
set -Eeuo pipefail
cd "$(dirname "$0")/.."
image="${DSP_IMAGE:-bidhelm:local}"
name="bidhelm-smoke-$$"
volume="$name-data"
port="${SMOKE_PORT:-18080}"
export ADMIN_PASSWORD="$(openssl rand -hex 16)"
export SMOKE_URL="http://127.0.0.1:$port"
cleanup(){ docker rm -f "$name" >/dev/null 2>&1 || true; docker volume rm "$volume" >/dev/null 2>&1 || true; }
trap cleanup EXIT
# Only resources created by this invocation are used and removed.
docker volume create "$volume" >/dev/null
openssl rand -hex 32 | docker run --rm -i --user root -v "$volume:/data" -v "$PWD/internal/runtime/testdata:/fixtures:ro" "$image" /bin/bash -c 'umask 077; cat > /data/internal-token; cp /fixtures/GeoIP2-Country-Test.mmdb /data/country.mmdb; chown -R 10001:10001 /data'
printf '%s\n' "$ADMIN_PASSWORD" | docker run --rm -i -v "$volume:/data" "$image" /app/backoffice init smoke
start(){ docker run -d --name "$name" -e PUBLIC_URL="$SMOKE_URL" -v "$volume:/data" -p "127.0.0.1:$port:8080" "$image" >/dev/null; }
ready(){ for ((i=0;i<40;i++)); do if curl -fsS "$SMOKE_URL/health/ready" >/dev/null 2>&1; then return; fi; sleep 1; done; docker logs "$name"; return 1; }
start
ready
INGRESS_LOADCHECK=1 python3 scripts/smoke.py
docker exec "$name" /app/metrics.sh | python3 -c 'import json,sys; m=json.load(sys.stdin); assert m["requests_completed_total"]>0 and m["outcomes_total"]["bid"]>0 and m["requests_in_flight"]==0; print("PASS: private metrics record traffic")'
[[ "$(docker exec "$name" curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8082/internal/metrics)" == 401 ]]
[[ "$(curl -s -o /dev/null -w '%{http_code}' "$SMOKE_URL/internal/metrics")" == 404 ]]
# Check coordinated shutdown, then recreate with the same persisted volume.
docker stop -t 15 "$name" >/dev/null
docker run --rm -v "$volume:/data" "$image" /app/backoffice backup /data/pre-update.db
docker rm "$name" >/dev/null
start
ready
python3 scripts/smoke.py --verify
# Exercise the ingress under a deliberately small quota using persisted data.
docker stop -t 15 "$name" >/dev/null
docker rm "$name" >/dev/null
docker run -d --name "$name" -e PUBLIC_URL="$SMOKE_URL" -e DSP_RPS=10 -e DSP_BURST=2 -v "$volume:/data" -p "127.0.0.1:$port:8080" "$image" >/dev/null
ready
before="$(docker exec "$name" /app/metrics.sh | python3 -c 'import json,sys; print(json.load(sys.stdin)["requests_started_total"])')"
python3 scripts/ingress-smoke.py
after="$(docker exec "$name" /app/metrics.sh | python3 -c 'import json,sys; print(json.load(sys.stdin)["requests_started_total"])')"
(( after - before > 0 && after - before < 201 ))
docker logs "$name" 2>&1 | python3 -c 'import sys; assert "limit=REJECTED upstream=-" in sys.stdin.read(); print("PASS: rejected requests bypass DSP and appear in ingress logs")'
# Unexpected termination of any service must stop the whole container.
for service in nginx backoffice dsp; do
 docker exec "$name" /bin/bash -c 'for p in /proc/[0-9]*/comm; do read -r comm < "$p" || continue; if [[ "$comm" == "$1" ]]; then pid=${p#/proc/}; pid=${pid%/comm}; kill -TERM "$pid"; exit 0; fi; done; exit 1' bash "$service"
 stopped=0
 for ((i=0;i<15;i++)); do
  if [[ "$(docker inspect -f '{{.State.Running}}' "$name")" == false ]]; then stopped=1; break; fi
  sleep 1
 done
 (( stopped == 1 )) || { echo "FAIL: container survived $service termination"; exit 1; }
 [[ "$(docker inspect -f '{{.State.ExitCode}}' "$name")" != 0 ]]
 echo "PASS: $service termination stops container"
 docker rm "$name" >/dev/null
 start
 ready
done
# Invalid environment must fail before starting services; CLI overrides still work.
if docker run --rm -e DSP_RPS=invalid "$image"; then echo 'FAIL: invalid rate accepted'; exit 1; fi
docker run --rm -e DSP_RPS=invalid "$image" /bin/true
