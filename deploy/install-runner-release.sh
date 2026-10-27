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
sandbox_archive=/opt/cppdefense/source/deploy/.cppdefense-sandbox.tar.gz.new
sandbox_checksum=/opt/cppdefense/source/deploy/.cppdefense-sandbox.sha256.new
sandbox_id=/opt/cppdefense/source/deploy/.cppdefense-sandbox.image-id.new
runner_home=/var/lib/cppdefense-runner
runner_uid=$(id -u cppdefense-runner)
sandbox_stage=
replaced=0
committed=0
sandbox_replaced=0
sandbox_previous=

# Serialize root-owned staging, image retagging and binary replacement.
exec 9>/run/lock/cppdefense-install-runner.lock
flock -n 9 || { echo 'Another runner installation is active' >&2; exit 1; }

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
  if [ -n "$sandbox_stage" ]; then
    rm -f "$sandbox_stage/image.tar.gz" "$sandbox_stage/sha256" "$sandbox_stage/image-id"
    rmdir "$sandbox_stage"
  fi
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

for artifact in "$sandbox_archive" "$sandbox_checksum" "$sandbox_id"; do
  test -f "$artifact"
  test ! -L "$artifact"
  test -s "$artifact"
done
install -d -m 0700 -o cppdefense-runner -g cppdefense-runner "/run/user/$runner_uid"
# Private staging prevents replacement of the upload after checksum validation.
# Root owns the directory; the runner can traverse/read but cannot modify it.
install -d -m 0711 -o root -g root /var/lib/cppdefense-releases
sandbox_stage=$(mktemp -d /var/lib/cppdefense-releases/sandbox.XXXXXXXX)
chown root:cppdefense-runner "$sandbox_stage"
chmod 0750 "$sandbox_stage"
install -o root -g cppdefense-runner -m 0440 "$sandbox_archive" "$sandbox_stage/image.tar.gz"
install -o root -g root -m 0400 "$sandbox_checksum" "$sandbox_stage/sha256"
install -o root -g root -m 0400 "$sandbox_id" "$sandbox_stage/image-id"
expected_checksum=$(cat "$sandbox_stage/sha256")
expected_id=$(cat "$sandbox_stage/image-id")
printf '%s\n' "$expected_checksum" | grep -Eq '^[0-9a-f]{64}$'
printf '%s\n' "$expected_id" | grep -Eq '^sha256:[0-9a-f]{64}$'
actual_checksum=$(sha256sum "$sandbox_stage/image.tar.gz" | cut -d ' ' -f 1)
if [ "$actual_checksum" != "$expected_checksum" ]; then
  echo 'Sandbox archive checksum mismatch; installation refused' >&2
  exit 1
fi
sandbox_previous=$(runner_podman image inspect --format '{{.Id}}' "$sandbox_image" 2>/dev/null || true)
runner_podman load --input "$sandbox_stage/image.tar.gz"
loaded_id=$(runner_podman image inspect --format '{{.Id}}' "$expected_id")
test "${loaded_id#sha256:}" = "${expected_id#sha256:}"
test "$(runner_podman image inspect --format '{{.Architecture}}' "$expected_id")" = amd64
test "$(runner_podman image inspect --format '{{.Os}}' "$expected_id")" = linux
runner_podman run --rm --pull never --network none "$expected_id" sh -lc \
  'command -v ninja && test -x /opt/cppdefense/timed.sh && test -r /opt/cppdefense/googletest/CMakeLists.txt && find /usr/lib -name GTestConfig.cmake | grep -q .'
# Change the production tag only after load, identity verification and smoke test.
sandbox_replaced=1
runner_podman tag "$expected_id" "$sandbox_image"

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
