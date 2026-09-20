# CppDefense

> **Source available for review. Proprietary software — not open source.**

Copyright (c) 2026 Zakharev Georgii. All rights reserved.

CppDefense is a C++23 console application for practicing university-style C++
lab defenses. It turns an existing CMake project into a timed exercise: the
application selects a meaningful code entity, hides its implementation and
checks the restored body by rebuilding the project and running its tests.

The original project is never modified. All preparation and validation happen
inside isolated working copies.

## Why this project exists

CppDefense is designed for students who can read completed C++ code but want
to practice reproducing it under defense-like conditions. Instead of asking a
fixed question, it analyzes the selected project and creates an exercise from
its real functions and types.

The result is useful both as a training tool and as a compact example of a
modern C++ application with explicit errors, filesystem safety, process
management, testing and layered architecture.

## What it provides

- automatic discovery of C and C++ source files;
- selection from the largest functions or all supported entities;
- deterministic function selection when an explicit 64-bit seed is supplied;
- support for functions, methods, constructors, operators, classes, structs
  and `enum class`;
- body-only answers: declarations and outer braces cannot be replaced;
- an isolated cached project and a separate temporary check workspace;
- UUID-scoped session workspaces instead of a shared `cache/current`;
- a headless `cpp-defense-worker` with a versioned JSON stdin/stdout protocol;
- real validation through CMake configure, build and CTest;
- retryable attempts within a monotonic time limit;
- hard deadlines for compiler and test processes;
- captured configure, build and test logs;
- a final report containing status, attempts and elapsed time;
- Linux, macOS and Windows CI coverage.

## Safety model

CppDefense keeps three project states separate:

```text
original project  →  masked session copy  →  temporary check copy
```

Only the temporary check copy receives the submitted body. Paths and symbolic
links are validated before cache cleanup, and external commands are started
without a command shell.

CppDefense does execute the selected project's CMake configuration, binaries
and tests. Projects should therefore be treated as trusted local code.

## Technical overview

The codebase is divided into reusable components:

- `cpp-defense-core` — parsing, deterministic selection, timer and content
  hashing without CLI or JSON dependencies;
- `application` — defense use cases and session lifecycle;
- `infrastructure` — files, parsing, workspaces and processes;
- `ui` — command parsing and terminal interaction.
- `worker` — JSON validation and safe workspace operations for
  Backend/Runner integration.

The lightweight parser preserves byte offsets and line endings while ignoring
comments, literals and preprocessor text during structural analysis. It is
purpose-built for exercise generation rather than intended as a replacement
for a complete compiler frontend.

## Project status

**Version: 2.0.0**

The complete console workflow is preserved and the 2.0 web platform is
included in the same repository. The project contains
104 CTest scenarios, including worker protocol calls from Python and Go,
deterministic selection, parallel session isolation, end-to-end retries,
process timeouts and path safety.

Current scope:

- C++23 compiler required to build CppDefense;
- CMake 3.20+ projects are supported as exercise targets;
- local CLI sessions are process-local and are not restored after restart;
- web defenses, attempts, compilation history and uploaded labs are persistent;
- worker preparation state is persisted inside its runner-provided session directory;
- highly macro-driven or exotic C++ syntax may be outside parser coverage.

## Documentation

- [Usage](docs/USAGE.md) — build and use the local CLI.
- [Architecture](docs/ARCHITECTURE.md) — components, data flow, invariants, and
  security boundaries.
- [Go backend](backend/README.md) — local setup, storage, migrations, and tests.
- [CppDefense 2.0 deployment](docs/DEPLOYMENT_2.0.md) — two-VM production setup,
  backup, restore, and operations.
- [UI preview](docs/UI_PREVIEW.html) — standalone mock page that can be opened
  directly in a browser without PostgreSQL, OAuth, or the Go server.
- [Contracts](contracts/README.md) — versioned ZIP, HTTP, and worker interfaces.
- [Roadmap](docs/ROADMAP.md) — implemented scope and next milestones.
- [Security policy](SECURITY.md) and [release guide](docs/RELEASING.md).

## License

Current licensing terms are in the [CppDefense Proprietary Source-Available
License](LICENSE). Repository viewing and GitHub forking for evaluation are
permitted, as is use of an official hosted service under its user terms.
Reuse, redistribution and self-hosting require written permission, except
where an earlier license already grants those rights.

Previously published MIT versions retain their MIT permissions. See the
[license transition record](docs/LICENSING.md).
