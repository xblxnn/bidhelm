# Operations

## Installation settings

The installer supports Ubuntu 24.04 and Debian 12/13 on x86_64 with systemd.
Run it as root, using a downloaded script so interactive prompts can read the
terminal. The default public URL is `http://<server-ip>` on port 80.

| Variable | Default | Meaning |
| --- | --- | --- |
| `DSP_IMAGE` | `ghcr.io/xblxnn/bidhelm:0.1.0` | Published image tag or digest |
| `DSP_PORT` | `80` | Host TCP port, 1–65535 |
| `DSP_BIND_IP` | `0.0.0.0` | Host IPv4 binding |
| `DSP_IP` | First host address | Advertised address or hostname |
| `DSP_HOME` | `/opt/bidhelm` | Absolute installation directory |
| `ADMIN_LOGIN` | Interactive prompt | Administrator login for a new installation |
| `MMDB_FILE` | Bundled database | Path to your country MMDB on the host |

`DSP_HOME/data` contains the SQLite database and internal token. `app.env`
contains the advertised URL. `initial-access.txt` contains initial credentials
and is root-readable only. It does not track later password changes.

The updater discovers the existing data path and networking from the container.
Do not use install settings to move an existing installation. Legacy
`dsp-compact` containers are replaced by `bidhelm` using the same data and ports.

## HTTPS and access

For an HTTPS reverse proxy, install with `DSP_BIND_IP=127.0.0.1` and a spare
`DSP_PORT`. Configure your proxy to forward to that address. Set `PUBLIC_URL`
in the installation's `app.env` to the external HTTPS URL, then recreate the
container using the same image, data mount, binding and environment file.
The setting enables secure cookies and correct source URLs. Make sure the SSP
can reach `/source/{id}` and limit administrative access at your proxy/firewall.

The public listener serves both the backoffice and source endpoints.
`/internal/*` returns 404 on the public port. Internal configuration and metrics
are available only inside the container with a bearer token.

## Health and logs

```sh
sudo docker ps --filter name=bidhelm
sudo docker logs --tail 100 bidhelm
sudo docker exec bidhelm curl -fsS http://127.0.0.1:8080/health/ready
sudo docker exec bidhelm /app/metrics.sh
```

`/health/live` checks the web process. `/health/ready` requires a valid MMDB and
an applied configuration revision. Installation and update fail if readiness
does not succeed within the bounded startup wait. Data is retained for diagnosis.
If the first startup fails, retrieve initial credentials from the root-only
`initial-access.txt` file before retrying; an existing administrator is preserved.

## Country database

Default: bundled DB-IP Country Lite. A `/data/country.mmdb` override takes
priority. An explicit `MMDB_PATH` takes priority over both. A corrupt override
causes readiness failure; it is not silently replaced with the bundled database.
Compatible data contains `country.iso_code`. Restart after replacing the file.
For a default installation, install an operator-provided file with:

```sh
sudo install -o 10001 -g 10001 -m 600 /path/to/country.mmdb /opt/bidhelm/data/country.mmdb
sudo docker restart bidhelm
```

You are responsible for the license and freshness of your own data.

## Password reset

The new password must be 12–72 bytes. Existing sessions are revoked.

```sh
read -rs password
printf '%s\n' "$password" | sudo docker exec -i bidhelm /app/backoffice reset-password
unset password
```

## Backup

Updates automatically stop writers and create a consistent SQLite backup using
the old image. They save the old image ID, port mapping and environment beside
the backup in the installation's `backups/` directory. Keep the old image locally
until you no longer need rollback; do not prune it immediately after an update.

For a manual consistent database backup:

```sh
sudo docker exec bidhelm /app/backoffice backup /data/manual-backup.db
```

Copy backups off the server securely together with the internal token and
required configuration. Database backups contain sensitive campaign and account
data. Do not commit them to Git or attach them to public issues.

## Restore

Restore the database **and** its matching image. Starting an older binary against
a newer database is not a supported rollback. Stop writers first. The following
example assumes the default installation directory; use the discovered legacy
directory for an older installation.

Run in a root shell (`sudo -i`). Choose the exact backup printed by the updater:

```sh
set -eu
bidhelm_root=/opt/bidhelm
backup="$bidhelm_root/backups/REPLACE_WITH_BACKUP_FILENAME.db"
old_image=$(cat "$backup.image")
publication=$(cat "$backup.port")
docker image inspect "$old_image" >/dev/null
test -r "$backup"
test -r "$backup.env"
test -r "$bidhelm_root/data/internal-token"
docker stop bidhelm
# Retain the failed database, including WAL/SHM, separately before replacement.
saved="$bidhelm_root/failed-data-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -m 700 "$saved"
cp -a "$bidhelm_root/data/." "$saved/"
docker rm bidhelm
rm -f "$bidhelm_root/data/app.db-wal" "$bidhelm_root/data/app.db-shm"
install -o 10001 -g 10001 -m 600 "$backup" "$bidhelm_root/data/app.db"
install -m 600 "$backup.env" "$bidhelm_root/app.env"
docker run -d --name bidhelm --restart unless-stopped \
  --env-file "$bidhelm_root/app.env" -p "$publication" \
  -v "$bidhelm_root/data:/data" "$old_image"
docker exec bidhelm curl --retry 30 --retry-delay 1 --retry-all-errors \
  --max-time 2 -fsS http://127.0.0.1:8080/health/ready
```

If a failed update removed the container before replacement, skip `docker stop`
and `docker rm` after confirming no container is writing to this data directory.
For backups made by older updaters without `.port` and `.env` files, supply the
previous port mapping and retained environment explicitly.
