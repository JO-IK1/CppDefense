#!/bin/sh
set -eu
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
if [ -f "$script_dir/production.env" ]; then
  set -a
  . "$script_dir/production.env"
  set +a
fi
origin=${1:-${CPPDEFENSE_PUBLIC_ORIGIN:-http://127.0.0.1:8080}}
curl --fail --silent --show-error --max-time 10 "$origin/health/live" >/dev/null
curl --fail --silent --show-error --max-time 10 "$origin/health/ready" >/dev/null
available_kb=$(df -Pk / | awk 'NR==2 {print $4}')
if [ "$available_kb" -lt 5242880 ]; then
  printf 'warning: less than 5 GiB free on root filesystem\n' >&2
  exit 2
fi
printf 'CppDefense healthy\n'
