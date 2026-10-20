import json
import hashlib
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
import zipfile


class CloneLabTest(unittest.TestCase):
    def test_exports_exact_commit_and_empty_test_fixtures(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            repository = root / "repository"
            repository.mkdir()
            def git(*args):
                return subprocess.check_output(["git", "-C", str(repository), *args], text=True).strip()
            git("init", "--quiet")
            git("config", "user.email", "test@example.invalid")
            git("config", "user.name", "Clone test")
            (repository / "CMakeLists.txt").write_text("cmake_minimum_required(VERSION 3.20)\n")
            (repository / "empty.txt").touch()
            git("add", ".")
            git("commit", "--quiet", "-m", "fixture")
            git("branch", "deadline_0")
            sha = git("rev-parse", "HEAD")
            environment = dict(os.environ, GIT_CONFIG_COUNT="2",
                GIT_CONFIG_KEY_0="url." + repository.as_uri() + ".insteadOf",
                GIT_CONFIG_VALUE_0="git@github.com:org/labwork1-student.git",
                GIT_CONFIG_KEY_1="protocol.file.allow", GIT_CONFIG_VALUE_1="always")
            output = root / "export"
            script = Path(__file__).with_name("clone-lab.py")
            subprocess.run(["python3", str(script), "org/labwork1-student", "deadline_0", "--output", str(output)], env=environment, check=True, capture_output=True)
            metadata = json.loads((output / "labwork1-student-deadline_0.json").read_text())
            self.assertEqual(metadata["commit_sha"], sha)
            self.assertEqual(metadata["version_label"], "deadline_0")
            self.assertEqual(metadata["archive_sha256"], hashlib.sha256((output / "labwork1-student-deadline_0.zip").read_bytes()).hexdigest())
            with zipfile.ZipFile(output / "labwork1-student-deadline_0.zip") as archive:
                self.assertEqual(archive.read("empty.txt"), b"")
                self.assertIn("CMakeLists.txt", archive.namelist())
                self.assertNotIn(".git/config", archive.namelist())


if __name__ == "__main__":
    unittest.main()
