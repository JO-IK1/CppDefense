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
replaced=0
committed=0

cleanup() {
  status=$?
  set +e
  rm -f "$runner_pending" "$worker_pending"
  if [ "$replaced" -eq 1 ] && [ "$committed" -eq 0 ]; then
    install -o root -g root -m 0755 "$runner_backup" "$runner_pending"
    install -o root -g root -m 0755 "$worker_backup" "$worker_pending"
    mv -f "$runner_pending" "$runner_target"
    mv -f "$worker_pending" "$worker_target"
    systemctl restart "$service" || true
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
