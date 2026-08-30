# CppDefense

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
- support for functions, methods, constructors, operators, classes, structs
  and `enum class`;
- body-only answers: declarations and outer braces cannot be replaced;
- an isolated cached project and a separate temporary check workspace;
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

The codebase is divided into four layers:

- `core` — domain values, state and typed errors;
- `application` — defense use cases and session lifecycle;
- `infrastructure` — files, parsing, workspaces and processes;
- `ui` — command parsing and terminal interaction.

The lightweight parser preserves byte offsets and line endings while ignoring
comments, literals and preprocessor text during structural analysis. It is
purpose-built for exercise generation rather than intended as a replacement
for a complete compiler frontend.

## Project status

**Version: 1.0.0**

The complete console workflow is implemented. The project currently contains
101 CTest scenarios, including end-to-end retries, process timeouts, path
safety, parser errors and EOF finalization.

Current scope:

- C++23 compiler required to build CppDefense;
- CMake 3.20+ projects are supported as exercise targets;
- active sessions are process-local and are not restored after restart;
- highly macro-driven or exotic C++ syntax may be outside parser coverage.

## Documentation

- [Usage guide](docs/USAGE.md) — requirements, building, launching, commands
  and the complete user workflow;
- [Architecture](docs/ARCHITECTURE.md) — internal processing pipeline, state
  transitions, parser design and system invariants;
- [Learning map](docs/LEARNING.md) — C++ concepts demonstrated by each module;
- [Roadmap](docs/ROADMAP.md) — completed scope and possible future work;
- [Release guide](docs/RELEASING.md) — packaging and release checklist.

## License

CppDefense is available under the [MIT License](LICENSE).
