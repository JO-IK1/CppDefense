# CppDefense

CppDefense is a C++23 console application for practicing university-style C++
lab defenses. It copies a project into an isolated workspace, finds code
entities, selects one of the largest candidates, hides its implementation and
asks the user to restore it under a time limit.

The original project is never modified.

## Status

**Version: v1.0.0**

The console workflow is complete:

```text
source project
    ↓
isolated cache
    ↓
scan + parse
    ↓
top-N candidates
    ↓
random selected entity
    ↓
result.txt with signature
    +
masked cached project
    ↓
user edits result.txt
    ↓
check creates temporary project copy
    ↓
patch solution → CMake configure → build → CTest
    ↓
pass / compiler log / test log
```

## Main features

- interactive CLI with configurable project path, candidate count and timer;
- safe isolated workspace under `cache/current`;
- recursive C/C++ source discovery;
- lightweight C++ lexical and structural parser;
- support for functions, classes, structs and `enum class`;
- exact source/body byte offsets and line ranges;
- top-N selection with a fixed-capacity min-heap;
- random target selection with deterministic seeded mode for tests;
- source masking that preserves file size, line endings and offsets;
- `result.txt` generated from the original entity signature;
- temporary check workspace so user code is never written to the masked cache;
- CMake configure/build and CTest execution with captured logs;
- retryable checks until success or timeout;
- monotonic defense timer based on `std::chrono::steady_clock`;
- final `defense_result.txt` with attempts, elapsed time, result and last log;
- explicit typed errors throughout the pipeline;
- multi-platform CI and 89 CTest scenarios.

## Requirements

- C++23 compiler;
- CMake 3.20+;
- CTest (included with CMake).

CppDefense currently checks **CMake projects**. Other build systems can be
added behind `BuildRunner` later.

## Build

```bash
cmake -S . -B build
cmake --build build
ctest --test-dir build --output-on-failure
```

Run:

```bash
./build/cpp-defense
```

or start with a project already selected:

```bash
./build/cpp-defense ./examples/labwork_simple -n 5 -t 10
```

## Quick start

```text
CppDefense CLI
Version: 1.0.0
Project path: "./examples/labwork_simple"
Function candidates: 5
Timer: 10 minutes
Mode: functions only

> start
Starting defense...
Defense started.
Source files found: ...
Entities found: ...
Candidates retained: 5
Selected: [function] CalculateStatistics
Cached project: .../cache/current/project/labwork_simple
Edit this file: .../cache/current/result.txt
Final report: .../cache/current/defense_result.txt
Time remaining: 9:59
```

`result.txt` contains the selected signature and an empty body, for example:

```cpp
 double CalculateStatistics(const std::vector<int>& values) {

}
```

Restore the entity in that text file, then run:

```text
> check
Checking solution...
Attempt: 1
Configure: OK
Build: OK
Tests: FAILED
...
Check failed. Fix result.txt and run --check again.
```

After correcting the answer:

```text
> check
Checking solution...
Attempt: 2
Configure: OK
Build: OK
Tests: OK
Defense passed.
```

## Interactive commands

```text
start, -s, --start         Start or restart a defense session
check, build, -c, --check  Build and test result.txt
info, i                    Show selected entity and paths
time                       Show remaining time
-p, --path <directory>     Select a project directory
-n, --functions <count>    Change candidate count (1..50)
-t, --timer <minutes>      Change timer (1..180)
    --functions-only       Functions only (default)
    --all                  Functions, classes, structs and enum class
help, -h, --help           Show help
quit, q, -e, --exit        Finish the session and exit
```

A failed build or failed tests do **not** end the session. The attempt counter
is incremented and the user can edit `result.txt` and check again while time
remains.

## Runtime workspace

For a project named `labwork_simple`:

```text
cache/current/
├── project/
│   └── labwork_simple/       # permanent masked copy for this session
├── logs/
│   ├── configure.log
│   ├── build.log
│   └── tests.log
├── metadata/
├── result.txt                # user edits this
└── defense_result.txt        # final report
```

During `check`, an additional `cache/current/check/` directory is created and
removed automatically.

## Processing pipeline

```text
CliApp
  ↓
DefenseSession
  ├─ DefenseService
  │    ├─ WorkspaceCache
  │    └─ ProjectScanner
  ├─ SourceFileRepository
  ├─ SimpleSourceParser
  ├─ CandidatePicker
  │    └─ FixedPriorityQueue
  ├─ ResultFile
  ├─ FileMasker
  ├─ DefenseTimer
  └─ Check
       ├─ CheckWorkspace
       ├─ FilePatcher
       ├─ BuildRunner
       └─ DefenseResultWriter
```

## Architecture

The project follows four layers:

```text
core/
application/
infrastructure/
ui/
```

### `core`

Domain models and typed errors. It contains no console or filesystem
orchestration.

Examples:

```text
CodeEntityInfo
FixedPriorityQueue
Workspace
DefenseStatus
DefenseResult
BuildResult
CacheError / ParseError / PickerError / ...
```

### `application`

Use cases and session state:

```text
DefenseService
DefenseSession
DefenseTimer
CandidatePicker
```

### `infrastructure`

Filesystem, parsing and process execution:

```text
WorkspaceCache
ProjectScanner
SourceFileRepository
SimpleSourceParser
FileMasker
ResultFile
CheckWorkspace
FilePatcher
BuildRunner
DefenseResultWriter
```

### `ui`

Console interaction only:

```text
CliApp
CommandParser
```

`main.cpp` only creates and runs the CLI application.

## Supported C++ entities

The lightweight parser currently recognizes:

- free functions;
- member functions and qualified methods;
- constructors/destructors;
- operators and friend operators;
- classes;
- structs;
- `enum class`.

It masks comments, string/character/raw-string literals and preprocessor
content before structural analysis, and validates matching braces and
parentheses.

This is intentionally not a full C++ compiler frontend. Highly macro-driven
or exotic C++ syntax may be outside v1 parser coverage.

## Candidate selection

By default CppDefense keeps the five largest functions by body line count.
The count can be changed with `-n`.

Instead of sorting all `M` entities, `CandidatePicker` uses a fixed-capacity
min-heap:

```text
Time:   O(M log N)
Memory: O(N)
```

The smallest retained candidate stays at the heap root, so a stronger incoming
candidate can replace it immediately.

## Testing

The v1 test suite contains **89 CTest scenarios** covering:

- workspace creation and safety checks;
- recursive scanner behavior and symlink handling;
- byte-exact source reading and CRLF preservation;
- lexer/parser entities, offsets, operators and malformed input;
- fixed-capacity priority queue behavior;
- candidate filtering, top-N retention and deterministic RNG;
- masking and result-file generation;
- solution patching without modifying cached sources;
- deterministic timer expiration;
- real CMake configure/build/CTest success and failure paths;
- end-to-end defense start, failed attempt, retry, success and timeout.

Run all tests:

```bash
ctest --test-dir build --output-on-failure
```

## Example project

`examples/labwork_simple` is a small standalone CMake project with several
functions of different sizes and CTest tests. It is intended for manual demos
and smoke testing:

```bash
./build/cpp-defense ./examples/labwork_simple -n 5 -t 10
```

## CI/CD

GitHub Actions builds and tests the project on Linux, macOS and Windows.
Version tags matching `v*` are packaged as release artifacts.

## Limitations of v1

- build execution currently targets CMake projects;
- parsing is lightweight rather than Clang/AST-based;
- the timer is enforced when the CLI processes the next command; no background
  thread interrupts terminal input;
- session state is process-local; restarting CppDefense starts a new session.

These constraints keep v1 dependency-free and focused on the educational C++
architecture.

## Documentation

- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — formal architecture,
  execution model, session lifecycle, source-masking strategy, validation flow,
  and system invariants;
- [`docs/LEARNING.md`](docs/LEARNING.md) — C++ topics practiced by each module;
- [`docs/ROADMAP.md`](docs/ROADMAP.md) — completed v1 scope and possible next
  versions.

## License

CppDefense is licensed under the [MIT License](LICENSE).

You are free to use, copy, modify, merge, publish, distribute, sublicense,
and/or sell copies of the software under the terms of the MIT License.
