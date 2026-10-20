#!/usr/bin/env python3
"""Import the exact batch selected in CppDefense, using local Git HTTPS access."""
import argparse
import json
from pathlib import Path
import re
import subprocess
import tempfile
import urllib.error
import urllib.request
from urllib.parse import urlparse
import uuid


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        raise ValueError("Unexpected redirect from CppDefense")


def latest_deadline(refs):
    branches = re.findall(r"refs/heads/(deadline_([0-9]+))$", refs, re.MULTILINE)
    if not branches:
        raise ValueError("В репозитории нет веток deadline_N")
    return max(branches, key=lambda value: (int(value[1]), value[0]))[0]


def request(site, ticket, path, data=None, content_type=None):
    headers = {"Authorization": "Bearer " + ticket}
    if content_type:
        headers["Content-Type"] = content_type
    req = urllib.request.Request(site + path, data=data, headers=headers)
    try:
        with urllib.request.build_opener(NoRedirect()).open(req, timeout=180) as response:
            return json.load(response)
    except urllib.error.HTTPError as error:
        body = error.read().decode("utf-8", errors="replace")
        try:
            body = json.loads(body).get("title", body)
        except ValueError:
            pass
        raise ValueError("CppDefense HTTP " + str(error.code) + ": " + body) from error


def import_item(site, ticket, item):
    if not re.fullmatch(r"[A-Za-z0-9-]+/[A-Za-z0-9_.-]+", item["repository"]):
        raise ValueError("Некорректное имя репозитория")
    if not re.fullmatch(r"[0-9a-f-]{36}", item["id"]):
        raise ValueError("Некорректный ID работы")
    remote = "https://github.com/" + item["repository"] + ".git"
    branch = item["branch"]
    if branch == "latest":
        refs = subprocess.check_output(["git", "ls-remote", "--heads", remote], text=True, timeout=180)
        branch = latest_deadline(refs)
    if not re.fullmatch(r"deadline_[0-9]+", branch):
        raise ValueError("Некорректная ветка")
    with tempfile.TemporaryDirectory(prefix="cppdefense-import-") as folder:
        checkout = Path(folder) / "repository"
        archive = Path(folder) / "project.zip"
        print("  git clone: " + branch, flush=True)
        subprocess.run(["git", "clone", "--depth", "1", "--single-branch", "--branch", branch,
                        "--", remote, str(checkout)], check=True, timeout=600)
        sha = subprocess.check_output(["git", "-C", str(checkout), "rev-parse", "HEAD"], text=True).strip()
        subprocess.run(["git", "-C", str(checkout), "archive", "--format=zip", "--output=" + str(archive), sha], check=True)
        if archive.stat().st_size > 100 * 1024 * 1024:
            raise ValueError("ZIP превышает 100 МБ")
        boundary = "CppDefense" + uuid.uuid4().hex
        body = bytearray()
        for name, value in (("branch", branch), ("commit_sha", sha)):
            body.extend(("--" + boundary + '\r\nContent-Disposition: form-data; name="' + name + '"\r\n\r\n' + value + "\r\n").encode())
        body.extend(("--" + boundary + '\r\nContent-Disposition: form-data; name="archive"; filename="project.zip"\r\nContent-Type: application/zip\r\n\r\n').encode())
        body.extend(archive.read_bytes())
        body.extend(("\r\n--" + boundary + "--\r\n").encode())
        result = request(site, ticket, "/api/v1/local-import/items/" + item["id"] + "/archive", bytes(body), "multipart/form-data; boundary=" + boundary)
        print("  Загружено, ожидает подтверждения: " + result["import_id"], flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--site", required=True)
    parser.add_argument("--ticket", required=True)
    args = parser.parse_args()
    site = args.site.rstrip("/")
    parsed = urlparse(site)
    if parsed.scheme != "https" or not parsed.netloc or parsed.username or parsed.password or parsed.path or parsed.query or parsed.fragment:
        parser.error("site must be an HTTPS origin")
    plan = request(site, args.ticket, "/api/v1/local-import/plan")
    failed = 0
    for index, item in enumerate(plan["items"], 1):
        print(f"[{index}/{len(plan['items'])}] {item['repository']}", flush=True)
        try:
            import_item(site, args.ticket, item)
        except (ValueError, OSError, subprocess.SubprocessError, urllib.error.URLError) as error:
            failed += 1
            print("  Ошибка: " + str(error), flush=True)
    print(f"Завершено: {len(plan['items']) - failed} успешно, {failed} с ошибкой.")
    if failed:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
