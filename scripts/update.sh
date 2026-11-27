#!/usr/bin/env bash
set -Eeuo pipefail
export LC_ALL=C LANG=C
umask 077
[[ $EUID -eq 0 ]] || { echo 'Run as root'; exit 1; }
: "${DSP_IMAGE:?Set DSP_IMAGE to the new image tag or digest}"
container=bidhelm
if ! docker container inspect "$container" >/dev/null 2>&1 && docker container inspect dsp-compact >/dev/null 2>&1; then
 container=dsp-compact
fi
data_dir="$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Source}}{{end}}{{end}}' "$container")"
[[ "$data_dir" == */data ]] || { echo 'Cannot determine existing data directory; container unchanged'; exit 1; }
root="${data_dir%/data}"
docker image inspect "$DSP_IMAGE" >/dev/null 2>&1 || docker pull "$DSP_IMAGE"
# Pin the image before stopping the old container.
new="$(docker image inspect "$DSP_IMAGE" --format '{{.Id}}')"
old="$(docker inspect --format '{{.Image}}' "$container")"
# Read the existing binding before stopping anything. This also supports old installs.
binding="$(docker inspect --format '{{with index .HostConfig.PortBindings "8080/tcp"}}{{with index . 0}}{{.HostIp}}|{{.HostPort}}{{end}}{{end}}' "$container")"
IFS='|' read -r bind_ip port <<< "$binding"
[[ "$port" =~ ^[0-9]{1,5}$ ]] && ((10#$port >= 1 && 10#$port <= 65535)) || { echo 'Cannot determine existing host port for 8080/tcp; container unchanged'; exit 1; }
if [[ -z "$bind_ip" ]]; then
 publication="$port:8080"
elif [[ "$bind_ip" == *:* ]]; then
 publication="[$bind_ip]:$port:8080"
else
 publication="$bind_ip:$port:8080"
fi
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
backup="backup-$stamp-$$.db"
mkdir -p "$root/backups"
# Verify required configuration and backup location before stopping writers.
[[ -r "$root/app.env" ]] || { echo 'Cannot read app.env; container unchanged'; exit 1; }
[[ -w "$root/backups" ]] || { echo 'Cannot write backups; container unchanged'; exit 1; }
# Stop all writers before the snapshot; the old image performs a consistent SQLite backup.
docker stop "$container" >/dev/null
if ! docker run --rm -v "$root/data:/data" "$old" /app/backoffice backup "/data/$backup"; then docker start "$container" >/dev/null; exit 1; fi
if ! mv "$root/data/$backup" "$root/backups/$backup" ||
   ! printf '%s\n' "$old" > "$root/backups/$backup.image" ||
   ! printf '%s\n' "$publication" > "$root/backups/$backup.port" ||
   ! cp "$root/app.env" "$root/backups/$backup.env"; then
 docker start "$container" >/dev/null
 echo 'Cannot retain backup; original container restarted.' >&2
 exit 1
fi
failed(){ echo "Update failed. Restore: https://github.com/xblxnn/bidhelm/blob/v0.1.1/docs/operations.md#restore. Backup: $root/backups/$backup; old image: $old" >&2; }
trap failed ERR
docker rm "$container" >/dev/null
docker run -d --name bidhelm --restart unless-stopped --env-file "$root/app.env" -p "$publication" -v "$root/data:/data" "$new" >/dev/null
for ((i=0;i<30;i++)); do
 if docker exec bidhelm curl --max-time 2 -fsS http://127.0.0.1:8080/health/live >/dev/null 2>&1 &&
    docker exec bidhelm curl --max-time 2 -fsS http://127.0.0.1:8080/health/ready >/dev/null 2>&1; then echo "Updated. Backup: $root/backups/$backup"; exit 0; fi
 sleep 1
done
failed
exit 1
