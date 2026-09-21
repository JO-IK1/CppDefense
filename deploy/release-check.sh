#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_dir=$(CDPATH= cd -- "$script_dir/.." && pwd)
env_file=${1:-$script_dir/production.env}

if [ ! -f "$env_file" ]; then
  printf 'missing environment file: %s\n' "$env_file" >&2
  printf 'copy production.env.example, fill it, then rerun this command\n' >&2
  exit 2
fi

cd "$repo_dir"
go -C backend test -race ./...
go -C backend vet ./...
cmake -S . -B build-release-check -DCMAKE_BUILD_TYPE=Release
cmake --build build-release-check --parallel 2
ctest --test-dir build-release-check --output-on-failure --parallel 2
python3 tests/integration/worker/protocol_test.py build-release-check/cpp-defense-worker .
node --check backend/webassets/static/app.js

CPPDEFENSE_ENV_FILE="$env_file" docker compose --env-file "$env_file" \
  -f "$script_dir/production.compose.yaml" config --quiet
CPPDEFENSE_ENV_FILE="$env_file" docker compose --env-file "$env_file" \
  -f "$script_dir/production.compose.yaml" build backend
docker build -f "$script_dir/sandbox.Dockerfile" -t cppdefense-sandbox:release-check .

printf 'CppDefense 2.1.0 release checks passed\n'
