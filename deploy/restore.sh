#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  printf 'usage: CPPDEFENSE_RESTORE_CONFIRM=RESTORE ./restore.sh /absolute/path/to/backup\n' >&2
  exit 2
fi
if [ "${CPPDEFENSE_RESTORE_CONFIRM:-}" != RESTORE ]; then
  printf 'restore refused: set CPPDEFENSE_RESTORE_CONFIRM=RESTORE\n' >&2
  exit 2
fi

backup=$1
case "$backup" in
  /*) ;;
  *) printf 'backup path must be absolute\n' >&2; exit 2 ;;
esac
test -f "$backup/postgres.dump"
test -f "$backup/minio.tar.gz"
test -f "$backup/SHA256SUMS"
(cd "$backup" && sha256sum -c SHA256SUMS)

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
compose() {
  docker compose --env-file "$script_dir/production.env" -f "$script_dir/production.compose.yaml" "$@"
}

compose stop backend cloudflared minio
compose up -d postgres
compose exec -T postgres pg_restore --clean --if-exists --no-owner \
  --username=cppdefense --dbname=cppdefense < "$backup/postgres.dump"
compose --profile maintenance run -T --rm --no-deps volume-helper \
  'find /data -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +; tar -C /data -xzf -' \
  < "$backup/minio.tar.gz"
compose up -d
printf 'Restore completed from %s\n' "$backup"
