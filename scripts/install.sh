#!/usr/bin/env bash
# Installation artifact. Not executed during local development.
set -Eeuo pipefail
export LC_ALL=C LANG=C
export DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=a
umask 077
[[ $EUID -eq 0 ]] || { echo 'Run as root'; exit 1; }
. /etc/os-release
[[ "$ID" == ubuntu || "$ID" == debian ]] || { echo 'Ubuntu/Debian required'; exit 1; }
[[ "$(uname -m)" == x86_64 ]] || { echo 'x86_64 required'; exit 1; }
: "${DSP_IMAGE:?Set DSP_IMAGE to the built/published image tag or digest}"
# Host publication settings; the application continues to listen on container port 8080.
for setting in DSP_RPS DSP_BURST; do
 value="${!setting:-}"
 [[ -z "$value" || "$value" =~ ^[1-9][0-9]{0,6}$ ]] || { echo "$setting must be an integer from 1 to 9999999"; exit 1; }
done
port="${DSP_PORT:-80}"
bind_ip="${DSP_BIND_IP:-0.0.0.0}"
[[ "$port" =~ ^[0-9]{1,5}$ ]] && ((10#$port >= 1 && 10#$port <= 65535)) || { echo 'DSP_PORT must be an integer from 1 to 65535'; exit 1; }
port=$((10#$port))
[[ "$bind_ip" =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ ]] || { echo 'DSP_BIND_IP must be an IPv4 address'; exit 1; }
IFS=. read -r ip_a ip_b ip_c ip_d <<< "$bind_ip"
for octet in "$ip_a" "$ip_b" "$ip_c" "$ip_d"; do
 ((10#$octet <= 255)) || { echo 'DSP_BIND_IP must be an IPv4 address'; exit 1; }
done
bind_ip="$((10#$ip_a)).$((10#$ip_b)).$((10#$ip_c)).$((10#$ip_d))"
command -v docker >/dev/null || { apt-get update; apt-get install -y docker.io; systemctl enable --now docker; }
command -v openssl >/dev/null || { apt-get update; apt-get install -y openssl; }
if docker container inspect bidhelm >/dev/null 2>&1 || docker container inspect dsp-compact >/dev/null 2>&1; then
 echo 'Container already exists; use update.sh to update. Data and credentials unchanged.'
 exit 0
fi
docker image inspect "$DSP_IMAGE" >/dev/null 2>&1 || docker pull "$DSP_IMAGE"
root="${DSP_HOME:-/opt/bidhelm}"
[[ "$root" == /* && "$root" != *:* && "$root" != *$'\n'* ]] || { echo 'DSP_HOME must be an absolute path without colons or newlines'; exit 1; }
mkdir -p "$root/data" "$root/backups"
[[ -f "$root/data/internal-token" ]] || openssl rand -hex 32 > "$root/data/internal-token"
ip="${DSP_IP:-}"
if [[ -z "$ip" ]]; then
 if [[ "$bind_ip" != 0.0.0.0 ]]; then ip="$bind_ip"; else ip="$(hostname -I | awk '{print $1}')"; fi
fi
[[ -n "$ip" ]] || { echo 'Set DSP_IP'; exit 1; }
[[ "$ip" != *$'\n'* && "$ip" != *$'\r'* ]] || { echo 'DSP_IP must not contain newlines'; exit 1; }
public="http://$ip"
[[ "$port" == 80 ]] || public="$public:$port"
if [[ -n "${MMDB_FILE:-}" ]]; then cp "$MMDB_FILE" "$root/data/country.mmdb"; fi
chown -R 10001:10001 "$root/data"
if ! docker run --rm -v "$root/data:/data" "$DSP_IMAGE" /app/backoffice has-admin; then
 login="${ADMIN_LOGIN:-}"
 if [[ -z "$login" ]]; then read -r -p 'Administrator login: ' login < /dev/tty; fi
 [[ -n "$login" ]] || { echo 'Login required'; exit 1; }
 password="$(openssl rand -hex 16)"
 printf '%s\n' "$password" | docker run --rm -i -v "$root/data:/data" "$DSP_IMAGE" /app/backoffice init "$login"
fi
# Keep initial credentials recoverable if startup fails. Never replace existing credentials.
if [[ -n "${password:-}" ]]; then
 printf 'Backoffice: %s\nLogin: %s\nPassword: %s\n' "$public" "$login" "$password" > "$root/initial-access.txt"
 chmod 600 "$root/initial-access.txt"
fi
printf 'PUBLIC_URL=%s\nDSP_RPS=%s\nDSP_BURST=%s\n' "$public" "${DSP_RPS:-3000}" "${DSP_BURST:-100}" > "$root/app.env"
docker run -d --name bidhelm --restart unless-stopped --env-file "$root/app.env" -p "$bind_ip:$port:8080" -v "$root/data:/data" "$DSP_IMAGE" >/dev/null
for ((i=0;i<30;i++)); do
 if docker exec bidhelm curl --max-time 2 -fsS http://127.0.0.1:8080/health/live >/dev/null 2>&1 &&
    docker exec bidhelm curl --max-time 2 -fsS http://127.0.0.1:8080/health/ready >/dev/null 2>&1; then
  printf 'Backoffice: %s\n' "$public"
  if [[ -n "${password:-}" ]]; then printf 'Login: %s\nPassword: %s\n' "$login" "$password"; fi
  if [[ -n "${password:-}" ]]; then printf 'Initial credentials saved to: %s/initial-access.txt (root only)\n' "$root"; fi
  exit 0
 fi
 sleep 1
done
echo "Startup or readiness failed; inspect docker logs bidhelm and /health/ready. Data retained at $root/data. Initial credentials, if created, are in $root/initial-access.txt."
exit 1
