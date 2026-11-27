#!/usr/bin/env bash
# Run this downloaded file on the target Ubuntu/Debian x86_64 server.
set -Eeuo pipefail
export LC_ALL=C LANG=C
export DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=a
umask 077
[[ $EUID -eq 0 ]] || { echo 'Run with sudo bash bootstrap.sh'; exit 1; }
. /etc/os-release
[[ "$ID" == ubuntu || "$ID" == debian ]] || { echo 'Ubuntu/Debian required'; exit 1; }
[[ "$(uname -m)" == x86_64 ]] || { echo 'x86_64 required'; exit 1; }
export DSP_IMAGE="${DSP_IMAGE:-ghcr.io/xblxnn/bidhelm:0.1.1}"
command -v docker >/dev/null || { apt-get update; apt-get install -y docker.io; systemctl enable --now docker; }
docker info >/dev/null
if docker container inspect bidhelm >/dev/null 2>&1 || docker container inspect dsp-compact >/dev/null 2>&1; then
 echo 'BidHelm is already installed. Use update.sh; existing data and credentials are unchanged.'
 exit 0
fi
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
# Public releases must work without the operator's registry credentials.
export DOCKER_CONFIG="$work/docker"
mkdir -p "$DOCKER_CONFIG"
docker pull "$DSP_IMAGE" || { echo "Cannot download $DSP_IMAGE. Check the version, network and public package access. No installation changes were made." >&2; exit 1; }
# Run the host installer shipped with this exact image, without downloading another version.
image_id="$(docker image inspect "$DSP_IMAGE" --format '{{.Id}}')"
docker run --rm --entrypoint cat "$image_id" /app/install.sh > "$work/install.sh"
[[ -s "$work/install.sh" ]] || { echo 'Image has no installer'; exit 1; }
export DSP_IMAGE="$image_id"
bash "$work/install.sh"
