"""Run the real installer with isolated filesystem paths and mocked OS services."""
import hashlib
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


SHIM = r'''#!/usr/bin/env python3
import os, pathlib, shutil, subprocess, sys
name = pathlib.Path(sys.argv[0]).name
args = sys.argv[1:]
root = pathlib.Path(os.environ["RELEASE_TEST_ROOT"])
with (root / "calls").open("a") as log:
    log.write(name + " " + " ".join(args) + "\n")
if name == "id":
    print("1234")
elif name == "flock":
    import fcntl
    fcntl.flock(int(args[-1]), fcntl.LOCK_EX | fcntl.LOCK_NB)
elif name == "chown":
    pass
elif name == "install":
    mode = 0o755
    directories = False
    paths = []
    while args:
        arg = args.pop(0)
        if arg in ("-o", "-g"):
            args.pop(0)
        elif arg == "-m":
            mode = int(args.pop(0), 8)
        elif arg == "-d":
            directories = True
        else:
            paths.append(pathlib.Path(arg))
    if directories:
        for path in paths:
            path.mkdir(parents=True, exist_ok=True)
            path.chmod(mode)
    else:
        shutil.copyfile(*paths)
        paths[-1].chmod(mode)
elif name == "runuser":
    sys.exit(subprocess.call([str(root / "bin/podman")] + args[args.index("podman") + 1:]))
elif name == "podman":
    if args[:2] == ["image", "inspect"]:
        fmt, image = args[3], args[4]
        if fmt == "{{.Architecture}}":
            print(os.environ.get("IMAGE_ARCH", "amd64"))
        elif fmt == "{{.Os}}":
            print("linux")
        else:
            print("sha256:" + ("a" if image.startswith("localhost/") else os.environ.get("LOADED_ID_CHAR", "b")) * 64)
    elif args[0] == "run" and os.environ.get("FAIL_SMOKE"):
        sys.exit(1)
    elif args[0] == "load":
        if os.environ.get("FAIL_LOAD"):
            sys.exit(1)
        assert pathlib.Path(args[2]).read_bytes() == b"verified image archive"
    elif args[0] not in ("run", "tag"):
        sys.exit("unexpected Podman operation")
elif name == "systemctl":
    if args[0] == "restart":
        count = root / "restarts"
        previous = int(count.read_text()) if count.exists() else 0
        count.write_text(str(previous + 1))
        if os.environ.get("FAIL_RESTART") and previous == 0:
            sys.exit(1)
elif name == "sleep":
    pass
else:
    sys.exit("unexpected mock command")
'''


class RunnerReleaseTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.bin = self.root / "bin"
        self.bin.mkdir()
        for name in ("install", "runuser", "podman", "systemctl", "id", "chown", "sleep", "flock"):
            path = self.bin / name
            path.write_text(SHIM)
            path.chmod(0o755)
        source = Path(__file__).resolve().parents[1] / "install-runner-release.sh"
        script = source.read_text()
        for fixed in ("/opt/cppdefense", "/var/lib/cppdefense-runner", "/var/lib/cppdefense-releases", "/run/user", "/run/lock"):
            script = script.replace(fixed, str(self.root) + fixed)
        self.script = self.root / "installer.sh"
        self.script.write_text(script)
        self.deploy = self.root / "opt/cppdefense/source/deploy"
        self.targets = self.root / "opt/cppdefense/bin"
        self.deploy.mkdir(parents=True)
        self.targets.mkdir(parents=True)
        (self.root / "run/lock").mkdir(parents=True)
        for binary in ("cppdefense-runner", "cpp-defense-worker"):
            for path, value in ((self.targets / binary, "old"), (self.deploy / ("." + binary + ".new"), "new")):
                path.write_text(value)
                path.chmod(0o755)
        archive = self.deploy / ".cppdefense-sandbox.tar.gz.new"
        archive.write_bytes(b"verified image archive")
        (self.deploy / ".cppdefense-sandbox.sha256.new").write_text(hashlib.sha256(archive.read_bytes()).hexdigest() + "\n")
        (self.deploy / ".cppdefense-sandbox.image-id.new").write_text("sha256:" + "b" * 64 + "\n")

    def run_installer(self, **overrides):
        env = {**os.environ, "PATH": str(self.bin) + os.pathsep + os.environ["PATH"], "RELEASE_TEST_ROOT": str(self.root), **overrides}
        result = subprocess.run(["sh", str(self.script)], env=env, capture_output=True, text=True, timeout=15)
        self.calls = (self.root / "calls").read_text()
        self.assertNotIn("podman build", self.calls)
        staging = self.root / "var/lib/cppdefense-releases"
        self.assertFalse(staging.exists() and list(staging.iterdir()))
        return result

    def test_success_loads_verified_image_before_switching_tag_and_binaries(self):
        result = self.run_installer()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertLess(self.calls.index("podman load"), self.calls.index("podman run"))
        self.assertLess(self.calls.index("podman run"), self.calls.index("podman tag"))
        self.assertIn("--pull never --network none", self.calls)
        for binary in ("cppdefense-runner", "cpp-defense-worker"):
            self.assertEqual((self.targets / binary).read_text(), "new")

    def test_corrupt_archive_never_loads_or_restarts(self):
        (self.deploy / ".cppdefense-sandbox.tar.gz.new").write_bytes(b"corrupt")
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("checksum mismatch", result.stderr)
        self.assertNotIn("podman load", self.calls)
        self.assertNotIn("systemctl restart", self.calls)

    def test_invalid_image_id_rejected_before_load(self):
        (self.deploy / ".cppdefense-sandbox.image-id.new").write_text("wrong-id\n")
        self.assertNotEqual(self.run_installer().returncode, 0)
        self.assertNotIn("podman load", self.calls)

    def test_smoke_failure_does_not_change_production_tag(self):
        self.assertNotEqual(self.run_installer(FAIL_SMOKE="1").returncode, 0)
        self.assertNotIn("podman tag", self.calls)
        self.assertNotIn("systemctl restart", self.calls)
        self.assertEqual((self.targets / "cppdefense-runner").read_text(), "old")

    def test_loaded_identity_mismatch_does_not_change_production(self):
        self.assertNotEqual(self.run_installer(LOADED_ID_CHAR="c").returncode, 0)
        self.assertNotIn("podman run", self.calls)
        self.assertNotIn("podman tag", self.calls)

    def test_wrong_architecture_does_not_change_production(self):
        self.assertNotEqual(self.run_installer(IMAGE_ARCH="arm64").returncode, 0)
        self.assertNotIn("podman run", self.calls)
        self.assertNotIn("podman tag", self.calls)

    def test_load_failure_leaves_previous_runner_running(self):
        self.assertNotEqual(self.run_installer(FAIL_LOAD="1").returncode, 0)
        self.assertNotIn("systemctl restart", self.calls)
        self.assertNotIn("podman tag", self.calls)

    def test_restart_failure_rolls_back_both_binaries_and_image(self):
        self.assertNotEqual(self.run_installer(FAIL_RESTART="1").returncode, 0)
        for binary in ("cppdefense-runner", "cpp-defense-worker"):
            self.assertEqual((self.targets / binary).read_text(), "old")
        self.assertIn("podman tag sha256:" + "a" * 64, self.calls)
        self.assertEqual((self.root / "restarts").read_text(), "2")


if __name__ == "__main__":
    unittest.main()
