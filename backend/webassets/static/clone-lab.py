#!/usr/bin/env python3
"""Clone a teacher-accessible repository and export a commit for CppDefense."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess
import tempfile


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("repository", help="organization/repository")
    parser.add_argument("branch", help="deadline_0, deadline_1, ...")
    parser.add_argument("--output", type=Path, default=Path.cwd())
    parser.add_argument("--https", action="store_true", help="use HTTPS and your existing Git credential helper instead of SSH")
    args = parser.parse_args()
    if not re.fullmatch(r"[A-Za-z0-9-]+/[A-Za-z0-9_.-]+", args.repository):
        parser.error("repository must be organization/repository")
    if not re.fullmatch(r"deadline_[0-9]+", args.branch):
        parser.error("branch must be deadline_N")
    args.output.mkdir(parents=True, exist_ok=True)
    filename = args.repository.split("/")[1] + "-" + args.branch
    with tempfile.TemporaryDirectory(prefix="cppdefense-clone-") as temporary:
        checkout = Path(temporary) / "repository"
        remote = ("https://github.com/" if args.https else "git@github.com:") + args.repository + ".git"
        subprocess.run(["git", "clone", "--depth", "1", "--single-branch", "--branch", args.branch,
                        "--", remote, str(checkout)], check=True)
        sha = subprocess.check_output(["git", "-C", str(checkout), "rev-parse", "HEAD"], text=True).strip()
        archive = (args.output / (filename + ".zip")).resolve()
        metadata = (args.output / (filename + ".json")).resolve()
        if archive.exists() or metadata.exists():
            parser.error("output files already exist; select another --output directory")
        subprocess.run(["git", "-C", str(checkout), "archive", "--format=zip", "--output=" + str(archive), sha], check=True)
        digest = hashlib.sha256()
        with archive.open("rb") as stream:
            for chunk in iter(lambda: stream.read(1024 * 1024), b""):
                digest.update(chunk)
        metadata.write_text(json.dumps({"repository_full_name": args.repository,
                                       "version_label": args.branch, "commit_sha": sha,
                                       "archive_sha256": digest.hexdigest()}, indent=2) + "\n", encoding="utf-8")
        print("Upload both files in CppDefense:")
        print(archive)
        print(metadata)


if __name__ == "__main__":
    main()
