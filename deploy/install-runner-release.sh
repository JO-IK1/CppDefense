#!/bin/sh
set -eu

test "$#" -eq 0

runner_candidate=/opt/cppdefense/source/deploy/.cppdefense-runner.new
runner_target=/opt/cppdefense/bin/cppdefense-runner
runner_backup=/opt/cppdefense/bin/cppdefense-runner.rollback
runner_pending=/opt/cppdefense/bin/.cppdefense-runner.pending
worker_candidate=/opt/cppdefense/source/deploy/.cpp-defense-worker.new
worker_target=/opt/cppdefense/bin/cpp-defense-worker
worker_backup=/opt/cppdefense/bin/cpp-defense-worker.rollback
worker_pending=/opt/cppdefense/bin/.cpp-defense-worker.pending
service=cppdefense-runner.service
sandbox_image=localhost/cppdefense-sandbox:2.1.0
sandbox_dockerfile=/opt/cppdefense/source/deploy/sandbox.Dockerfile
runner_home=/var/lib/cppdefense-runner
runner_uid=$(id -u cppdefense-runner)
sandbox_context=$runner_home/sandbox-build-context
replaced=0
committed=0
sandbox_replaced=0
sandbox_previous=

runner_podman() {
  runuser -u cppdefense-runner -- env \
    HOME="$runner_home" \
    XDG_RUNTIME_DIR="/run/user/$runner_uid" \
    podman "$@"
}

cleanup() {
  status=$?
  set +e
  rm -f "$runner_pending" "$worker_pending"
  if [ "$replaced" -eq 1 ] && [ "$committed" -eq 0 ]; then
    install -o root -g root -m 0755 "$runner_backup" "$runner_pending"
    install -o root -g root -m 0755 "$worker_backup" "$worker_pending"
    mv -f "$runner_pending" "$runner_target"
    mv -f "$worker_pending" "$worker_target"
    if [ "$sandbox_replaced" -eq 1 ] && [ -n "$sandbox_previous" ]; then
      runner_podman tag "$sandbox_previous" "$sandbox_image"
    fi
    systemctl restart "$service" || true
  elif [ "$sandbox_replaced" -eq 1 ] && [ "$committed" -eq 0 ] && [ -n "$sandbox_previous" ]; then
    runner_podman tag "$sandbox_previous" "$sandbox_image"
  fi
  exit "$status"
}
trap cleanup EXIT HUP INT TERM

for path in \
  "$runner_candidate" \
  "$worker_candidate" \
  "$runner_target" \
  "$worker_target"
do
  test -f "$path"
  test ! -L "$path"
  test -x "$path"
done

test -f "$sandbox_dockerfile"
install -d -m 0700 -o cppdefense-runner -g cppdefense-runner "/run/user/$runner_uid"
install -d -m 0700 -o cppdefense-runner -g cppdefense-runner "$sandbox_context"
sandbox_previous=$(runner_podman image inspect --format '{{.Id}}' "$sandbox_image" 2>/dev/null || true)
runner_podman build \
  --file "$sandbox_dockerfile" \
  --tag "$sandbox_image" \
  "$sandbox_context"
sandbox_replaced=1
runner_podman run --rm --network none "$sandbox_image" sh -lc \
  'test -r /usr/src/googletest/CMakeLists.txt && test -r /usr/lib/x86_64-linux-gnu/cmake/GTest/GTestConfig.cmake'

install -o root -g root -m 0755 "$runner_target" "$runner_backup"
install -o root -g root -m 0755 "$worker_target" "$worker_backup"
install -o root -g root -m 0755 "$runner_candidate" "$runner_pending"
install -o root -g root -m 0755 "$worker_candidate" "$worker_pending"
replaced=1
mv -f "$runner_pending" "$runner_target"
mv -f "$worker_pending" "$worker_target"

if systemctl restart "$service"; then
  sleep 2
  if systemctl is-active --quiet "$service"; then
    committed=1
    rm -f "$runner_backup" "$worker_backup"
    exit 0
  fi
fi

echo "Runner deployment failed; restoring the previous runner and worker" >&2
exit 1
