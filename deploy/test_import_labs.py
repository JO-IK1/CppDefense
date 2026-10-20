import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

script = Path(__file__).resolve().parents[1] / "backend/webassets/static/import-labs.py"
spec = importlib.util.spec_from_file_location("import_labs", script)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class BatchImportTest(unittest.TestCase):
    def test_latest_deadline_is_numeric(self):
        self.assertEqual(module.latest_deadline("a\trefs/heads/deadline_9\nb\trefs/heads/deadline_10\nc\trefs/heads/main\n"), "deadline_10")
        with self.assertRaises(ValueError):
            module.latest_deadline("a\trefs/heads/main\n")

    def test_local_clone_automatically_uploads_latest_branch(self):
        with tempfile.TemporaryDirectory() as directory:
            repository = Path(directory)
            def git(*args):
                return subprocess.check_output(["git", "-C", directory, *args], text=True).strip()
            git("init", "--quiet")
            git("config", "user.email", "test@example.invalid")
            git("config", "user.name", "Batch import test")
            (repository / "CMakeLists.txt").write_text("cmake_minimum_required(VERSION 3.20)\n")
            git("add", ".")
            git("commit", "--quiet", "-m", "fixture")
            git("branch", "deadline_9")
            git("branch", "deadline_10")
            environment = dict(os.environ, GIT_CONFIG_COUNT="2",
                GIT_CONFIG_KEY_0="url." + repository.as_uri() + ".insteadOf",
                GIT_CONFIG_VALUE_0="https://github.com/org/labwork1-student.git",
                GIT_CONFIG_KEY_1="protocol.file.allow", GIT_CONFIG_VALUE_1="always")
            item = {"id":"0199a123-4567-7abc-8def-0123456789ab", "repository":"org/labwork1-student", "branch":"latest"}
            with patch.dict(os.environ, environment), patch.object(module, "request", return_value={"import_id":"uploaded"}) as upload:
                module.import_item("https://cppdefense.example", "ticket", item)
                args = upload.call_args.args
                self.assertEqual(args[2], "/api/v1/local-import/items/" + item["id"] + "/archive")
                self.assertIn(b"deadline_10", args[3])
                self.assertIn(git("rev-parse", "HEAD").encode(), args[3])
                self.assertIn(b"PK", args[3])


if __name__ == "__main__":
    unittest.main()
