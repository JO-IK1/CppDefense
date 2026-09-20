#!/bin/sh
set -eu

backup_root=${CPPDEFENSE_BACKUP_DIR:-/var/backups/cppdefense}
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
compose() {
  docker compose --env-file "$script_dir/production.env" -f "$script_dir/production.compose.yaml" "$@"
}
stamp=$(date -u +%Y%m%dT%H%M%SZ)
destination="$backup_root/$stamp"
install -d -m 0700 "$backup_root"
umask 077
mkdir "$destination"

compose exec -T postgres \
  pg_dump --format=custom --no-owner --username=cppdefense cppdefense > "$destination/postgres.dump"

# Stop object writes while copying MinIO's on-disk format. PostgreSQL stays
# available; the backend reports not-ready for the short storage maintenance.
compose stop minio
restart_minio() { compose start minio >/dev/null; }
trap restart_minio EXIT INT TERM
compose --profile maintenance run -T --rm --no-deps volume-helper \
  'tar -C /data -czf - .' > "$destination/minio.tar.gz"
compose start minio >/dev/null
trap - EXIT INT TERM

sha256sum "$destination/postgres.dump" "$destination/minio.tar.gz" > "$destination/SHA256SUMS"
chmod 0600 "$destination"/*
printf 'Backup created: %s\n' "$destination"
