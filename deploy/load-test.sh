#!/bin/sh
set -eu

origin=${1:?usage: ./load-test.sh https://cppdefense.example [requests] [concurrency]}
requests=${2:-1000}
concurrency=${3:-100}

case "$requests:$concurrency" in
  *[!0-9:]*|0:*|*:0) printf 'requests and concurrency must be positive integers\n' >&2; exit 2 ;;
esac

export CPPDEFENSE_LOAD_URL="${origin%/}/health/ready"
start=$(date +%s)
seq 1 "$requests" | xargs -P "$concurrency" -n 1 sh -c \
  'curl --fail --silent --show-error --max-time 15 "$CPPDEFENSE_LOAD_URL" >/dev/null'
elapsed=$(( $(date +%s) - start ))
if [ "$elapsed" -lt 1 ]; then elapsed=1; fi
printf '%s requests completed with concurrency %s in %ss (~%s req/s)\n' \
  "$requests" "$concurrency" "$elapsed" "$((requests / elapsed))"
