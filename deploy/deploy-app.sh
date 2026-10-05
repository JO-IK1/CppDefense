#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$script_dir"

test -s production.env
runner_candidate="$script_dir/.cppdefense-runner.new"
worker_candidate="$script_dir/.cpp-defense-worker.new"
cleanup() {
  rm -f "$runner_candidate" "$worker_candidate"
}
trap cleanup EXIT HUP INT TERM

test -f "$worker_candidate"
test ! -L "$worker_candidate"
test -x "$worker_candidate"

/usr/local/bin/go \
  -C "$script_dir/../backend" \
  build \
  -trimpath \
  -ldflags="-s -w" \
  -o "$runner_candidate" \
  ./cmd/runner
chmod 0755 "$runner_candidate"

compose() {
  docker compose \
    --env-file production.env \
    -f production.compose.yaml \
    -f caddy.compose.yaml \
    "$@"
}

rollback_image=cppdefense-backend:rollback
if docker image inspect cppdefense-backend:local >/dev/null 2>&1; then
  docker image tag cppdefense-backend:local "$rollback_image"
fi

compose build backend
compose up -d --no-build --pull missing postgres minio backend caddy

if ./healthcheck.sh; then
  sudo -n /usr/local/sbin/cppdefense-install-runner
  printf 'Deployment completed successfully\n'
  exit 0
fi

printf 'Deployment healthcheck failed; restoring the previous backend image\n' >&2
if docker image inspect "$rollback_image" >/dev/null 2>&1; then
  docker image tag "$rollback_image" cppdefense-backend:local
  compose up -d --no-build --pull never --force-recreate backend caddy
  ./healthcheck.sh || true
else
  printf 'No previous backend image is available for rollback\n' >&2
fi
exit 1
