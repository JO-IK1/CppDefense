#!/usr/bin/env python3

import json
import pathlib
import subprocess
import sys
import tempfile

try:
    import jsonschema
except ImportError:
    jsonschema = None


SESSION_ID = "0199a123-4568-7abc-8def-0123456789ab"
REQUEST_ID = "0199a123-4567-7abc-8def-0123456789ab"


def invoke(worker: str, workspace: pathlib.Path, request: str) -> dict:
    completed = subprocess.run(
        [worker, "--workspace", str(workspace)],
        input=request,
        text=True,
        capture_output=True,
        check=False,
        timeout=15,
    )
    if completed.returncode != 0:
        raise AssertionError(
            f"worker exit {completed.returncode}: {completed.stderr}"
        )
    lines = completed.stdout.splitlines()
    if len(lines) != 1:
        raise AssertionError("worker stdout must contain exactly one JSON line")
    return json.loads(lines[0])


def request(command: str, payload: dict) -> str:
    return json.dumps(
        {
            "protocol_version": "1.0",
            "request_id": REQUEST_ID,
            "session_id": SESSION_ID,
            "command": command,
            "payload": payload,
        }
    )

def validate_response(response: dict, schema: dict) -> None:
    if jsonschema is not None:
        jsonschema.Draft202012Validator(
            schema, format_checker=jsonschema.FormatChecker()
        ).validate(response)


def main() -> int:
    if len(sys.argv) != 3:
        raise SystemExit("usage: worker_protocol_test.py WORKER SOURCE_ROOT")
    worker = sys.argv[1]
    source_root = pathlib.Path(sys.argv[2])
    request_schema = json.loads(
        (source_root / "contracts/worker/v1/request.schema.json").read_text(
            encoding="utf-8"
        )
    )
    response_schema = json.loads(
        (source_root / "contracts/worker/v1/response.schema.json").read_text(
            encoding="utf-8"
        )
    )

    def make_request(command: str, payload: dict) -> str:
        encoded = request(command, payload)
        if jsonschema is not None:
            jsonschema.Draft202012Validator(
                request_schema, format_checker=jsonschema.FormatChecker()
            ).validate(json.loads(encoded))
        return encoded

    with tempfile.TemporaryDirectory(prefix="cpp-defense-worker-protocol-") as raw:
        workspace = pathlib.Path(raw)
        project = workspace / SESSION_ID / "project"
        project.mkdir(parents=True)
        (project / "CMakeLists.txt").write_text(
            "cmake_minimum_required(VERSION 3.20)\nproject(Sample LANGUAGES CXX)\n",
            encoding="utf-8",
        )
        (project / "main.cpp").write_text(
            "int Answer() {\n  return 42;\n}\n", encoding="utf-8"
        )

        analyzed = invoke(
            worker,
            workspace,
            make_request("analyze_project", {"project_root": "project"}),
        )
        assert analyzed["protocol_version"] == "1.0"
        assert analyzed["request_id"] == REQUEST_ID
        assert analyzed["session_id"] == SESSION_ID
        assert analyzed["command"] == "analyze_project"
        assert analyzed["status"] == "ok"
        assert analyzed["result"]["function_count"] == 1
        validate_response(analyzed, response_schema)

        prepared = invoke(
            worker,
            workspace,
            make_request(
                "prepare_defense",
                {"project_root": "project", "top_n": 1, "seed": "42"},
            ),
        )
        assert prepared["status"] == "ok"
        assert prepared["result"]["selected_index"] == 0
        assert len(prepared["result"]["candidates"]) == 1
        validate_response(prepared, response_schema)

        repeated = invoke(
            worker,
            workspace,
            make_request(
                "prepare_defense",
                {"project_root": "project", "top_n": 1, "seed": "42"},
            ),
        )
        assert repeated["result"] == prepared["result"]
        validate_response(repeated, response_schema)

        (workspace / SESSION_ID / "attempts").mkdir()
        selected = prepared["result"]["selected_function"]
        materialized = invoke(
            worker,
            workspace,
            make_request(
                "materialize_attempt",
                {
                    "project_root": "project",
                    "output_root": "attempts/one",
                    "selected_function": {
                        "function_name": selected["function_name"],
                        "file_path": selected["file_path"],
                        "signature_begin": selected["signature_begin"],
                        "body_begin": selected["body_begin"],
                        "body_end": selected["body_end"],
                        "source_sha256": selected["source_sha256"],
                    },
                    "answer": "\n  return 7;\n",
                },
            ),
        )
        assert materialized["status"] == "ok"
        output_source = (
            workspace
            / SESSION_ID
            / "attempts/one"
            / selected["file_path"]
        ).read_text(encoding="utf-8")
        assert "{\n  return 7;\n}" in output_source
        validate_response(materialized, response_schema)

        malformed = invoke(worker, workspace, "{not json")
        assert malformed["status"] == "error"
        assert malformed["error"]["code"] == "INVALID_JSON"
        assert malformed["request_id"] is None
        validate_response(malformed, response_schema)

        unsafe = invoke(
            worker,
            workspace,
            request("analyze_project", {"project_root": "../project"}),
        )
        assert unsafe["status"] == "error"
        assert unsafe["error"]["code"] == "INVALID_PATH"
        assert unsafe["request_id"] == REQUEST_ID
        validate_response(unsafe, response_schema)

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
