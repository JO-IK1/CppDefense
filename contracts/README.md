# CppDefense contracts

This directory is the versioned boundary between Backend, Runner, C++ worker and import producers.

- `manifests/`: ZIP manifest schemas and valid examples.
- `worker/v1/`: JSON stdin/stdout contract for `cpp-defense-worker`.
- `openapi/`: browser/API and private Runner HTTP contract.
- `tests/`: fixtures that must be rejected.
- `requirements-ci.txt`: pinned contract validators used by CI.

Contract changes are made before implementation changes. A breaking worker, manifest or HTTP change requires a new major contract version; existing v1 files are not silently repurposed.

## Validation

Install the pinned tools in an isolated Python environment, then run the same commands as `.github/workflows/contracts.yml`. CI verifies each JSON Schema against Draft 2020-12, validates positive and negative examples, and validates `openapi-v1.yaml` as OpenAPI 3.1.

Test examples contain synthetic identities and hashes only. Real student manifests, archives, logs and results are forbidden in this directory and the public Git history.
