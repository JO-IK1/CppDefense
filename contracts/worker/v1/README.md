# cpp-defense-worker protocol 1.0

## Transport

- One UTF-8 JSON request on stdin, one UTF-8 JSON response on stdout.
- stdout contains JSON only; diagnostics go to stderr and are size-limited by runner.
- Process handles one command and exits.
- Exit `0` means a schema-valid response was produced. Non-zero exit means worker crash/protocol failure and is infrastructure-retryable.
- JSON `status: error` is a handled response; retry is controlled by `error.retryable` and category.
- A handled error echoes validated envelope fields. If JSON or the envelope cannot be parsed safely, `request_id`, `session_id` and `command` are `null`.

## Commands

- `analyze_project`: scans a project and returns all supported candidates.
- `prepare_defense`: returns deterministic top-N and selected function for the supplied seed.
- `materialize_attempt`: copies the immutable input project into an output directory and substitutes the answer only if file digest and offsets still match.

All paths are relative to runner-provided workspace. Worker joins and canonicalizes each path, then verifies the result remains beneath the workspace root. Symlinks are rejected.

Offsets are byte offsets in the exact UTF-8/source byte sequence whose SHA-256 is supplied. `body_begin` points to `{`; `body_end` points one byte after matching `}`. The invariant is:

`signature_begin <= body_begin < body_end <= source_size`.

The answer contains only the body contents, without the function's outer braces, exactly as in CLI v1. Nested blocks inside the answer are permitted. Worker preserves the original outer braces and signature. Before materialization worker verifies `source_sha256`; mismatch returns `SOURCE_CHANGED`.

`seed` is encoded as a decimal string to avoid JSON/JavaScript precision loss. In addition to the schema pattern, both producer and worker must reject values above `18446744073709551615` (`uint64` maximum).

## Compatibility

- Major `1` is the compatibility boundary.
- A 1.x producer may add optional response fields only after schema 1.x update.
- Unknown commands, fields or protocol major versions are rejected.
- Go and C++ contract tests validate the same golden examples.
