#!/bin/sh
set -eu

test "$#" -eq 0

candidate=/opt/cppdefense/source/deploy/.cppdefense-runner.new
target=/opt/cppdefense/bin/cppdefense-runner
backup=/opt/cppdefense/bin/cppdefense-runner.rollback
pending=/opt/cppdefense/bin/.cppdefense-runner.pending
service=cppdefense-runner.service

cleanup() {
  rm -f "$pending"
}
trap cleanup EXIT HUP INT TERM

test -f "$candidate"
test ! -L "$candidate"
test -x "$candidate"
test -f "$target"

install -o root -g root -m 0755 "$target" "$backup"
install -o root -g root -m 0755 "$candidate" "$pending"
mv -f "$pending" "$target"

if systemctl restart "$service"; then
  sleep 2
  if systemctl is-active --quiet "$service"; then
    rm -f "$backup"
    exit 0
  fi
fi

install -o root -g root -m 0755 "$backup" "$pending"
mv -f "$pending" "$target"
systemctl restart "$service" || true
echo "Runner deployment failed; the previous binary was restored" >&2
exit 1
