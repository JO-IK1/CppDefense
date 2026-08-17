# CppDefense Architecture and Execution Model

## 1. Purpose and Scope

CppDefense is a C++23 console application designed to reproduce a university-style C++ laboratory defense workflow. The application prepares an isolated copy of a user project, discovers supported code entities, selects a defense target, removes its implementation from the cached project, and asks the user to restore the selected entity within a configured time limit.

The application then validates the submitted implementation by creating a temporary check workspace, patching the submitted code into that copy, configuring and building the project with CMake, and executing its CTest suite.

The original source project is never modified.

This document describes the architecture of the v1.0 console implementation and the execution flow of the main defense operations.

## 2. Architectural Overview

CppDefense follows a four-layer structure:

```text
core/
application/
infrastructure/
ui/
```

The layers are intentionally separated so that domain state and application rules do not depend on the console interface, filesystem details, or a future frontend implementation.

The high-level execution model is:

```text
main.cpp
   ↓
CliApp
   ↓
DefenseSession
   ├─ project preparation
   ├─ source analysis
   ├─ candidate selection
   ├─ result-file generation
   ├─ source masking
   ├─ timer management
   └─ solution checking
        ├─ temporary workspace creation
        ├─ source patching
        ├─ CMake configure
        ├─ build
        └─ CTest execution
```

## 3. Layer Responsibilities

### 3.1 `core`

The `core` layer contains domain models, reusable value types, generic containers, state enumerations, and typed error models.

Representative types include:

```text
CodeEntityInfo
FixedPriorityQueue
Workspace
DefenseStatus
DefenseResult
BuildResult
CacheError
ParseError
PickerError
```

The layer does not coordinate console interaction or filesystem workflows. Its purpose is to define the data contracts used by the rest of the system.

### 3.2 `application`

The `application` layer implements use cases and coordinates the defense workflow.

Primary components:

```text
DefenseService
DefenseSession
DefenseTimer
CandidatePicker
```

`DefenseSession` is the main application-level orchestrator. It owns the state of the active defense and delegates technical operations to smaller infrastructure components.

### 3.3 `infrastructure`

The `infrastructure` layer contains implementations that interact with files, directories, source text, build tools, and operating-system processes.

Primary components:

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

### 3.4 `ui`

The `ui` layer contains console-specific behavior:

```text
CliApp
CommandParser
```

`CliApp` interprets user commands, invokes `DefenseSession`, and prints results. Defense logic is intentionally not implemented directly in the CLI.

## 4. Program Entry Point

The executable starts in `apps/cli/main.cpp`.

The entry point creates a `CliApp`, supplies the standard input/output streams, resolves the CppDefense root path, and calls `Run()`.

The intended responsibility of `main.cpp` is deliberately small:

```text
resolve application root
        ↓
construct CliApp
        ↓
run application
```

This keeps application logic out of the entry point and preserves testability of the remaining components.

## 5. CLI Command Processing

`CommandParser` converts startup arguments and interactive input into typed command values. `CliApp` then dispatches the corresponding operation.

The primary interactive commands are:

```text
start
check
info
time
quit
```

Configuration commands can also modify the selected project path, candidate count, timer duration, and candidate selection mode.

The relationship is:

```text
raw user input
      ↓
CommandParser
      ↓
InteractiveCommandType
      ↓
CliApp
      ↓
DefenseSession operation
```

The CLI is therefore responsible for presentation and command routing, while `DefenseSession` remains responsible for defense behavior.

## 6. Defense Session State

`DefenseSession` owns the current defense state and coordinates all major operations.

The session uses `DefenseStatus`:

```text
kIdle
kPreparing
kWorkspaceReady
kActive
kChecking
kSuccess
kFailed
kExpired
kError
```

A normal successful lifecycle is:

```text
Idle
 ↓
Preparing
 ↓
WorkspaceReady
 ↓
Active
 ↓
Checking
 ↓
Success
```

A failed build or failed test run does not terminate the defense:

```text
Active
 ↓
Checking
 ↓
Active
```

This allows the user to modify `result.txt` and submit another check while time remains.

A timeout transitions the session to:

```text
Expired
```

An unrecoverable preparation or infrastructure error transitions the session to:

```text
Error
```

## 7. Start Workflow

`DefenseSession::Start()` initializes a new defense.

Before processing a new project, the previous session state is cleared:

```text
timer stopped
workspace reset
selected entity reset
attempt counter reset
last build log cleared
status = Preparing
```

The start operation then performs the following stages.

### 7.1 Workspace Preparation

`DefenseService` combines two operations:

```text
WorkspaceCache
ProjectScanner
```

`WorkspaceCache` validates the selected source project, creates a new isolated session directory, removes an older session if necessary, and recursively copies the source project into the CppDefense cache.

For a project named `labwork_simple`, the active cached project is located under:

```text
cache/current/project/labwork_simple/
```

All subsequent source analysis and masking operations use this cached copy rather than the original project.

### 7.2 Source Discovery

`ProjectScanner` recursively searches the cached project for supported C and C++ source files.

It also excludes directories that should not participate in source analysis, such as build output, version-control metadata, IDE data, and other configured exclusions.

The result is a collection of source-file paths used by the parser.

### 7.3 Source Reading

`SourceFileRepository` reads source files in binary mode.

Binary mode is important because source offsets are stored as byte offsets. Unintended line-ending conversion would invalidate those positions.

### 7.4 Source Parsing

`SimpleSourceParser` performs lightweight lexical and structural analysis rather than using a complete compiler frontend.

The parser pipeline conceptually consists of:

```text
source text
   ↓
lexical masking
   ↓
brace / parenthesis structure analysis
   ↓
entity recognition
   ↓
CodeEntityInfo[]
```

Comments, string literals, character literals, raw string literals, and preprocessor content are masked before structural entity discovery so that syntax-like text inside those regions does not produce false entities.

The v1 parser recognizes:

- free functions;
- member functions and qualified methods;
- constructors and destructors;
- operators and friend operators;
- classes;
- structs;
- scoped `enum class` declarations.

The parser is intentionally lightweight and is not intended to provide complete C++ grammar coverage.

## 8. `CodeEntityInfo`

Every discovered entity is represented by `CodeEntityInfo`.

The structure stores:

```text
type
name
file path
entity line range
body line range
entity byte range
body byte range
```

The most important source coordinates are:

```text
start_offset
end_offset
body_start_offset
body_end_offset
```

For example:

```cpp
int Sum(int a, int b) {
  return a + b;
}
```

Conceptually:

```text
start_offset
↓
int Sum(int a, int b) {
                       ↑ body_start_offset
  return a + b;
}
↑ body_end_offset / entity end region
```

These offsets allow masking and patching to operate on exact source ranges without searching again by function or type name. This also avoids ambiguity when overloads or repeated identifiers are present.

## 9. Candidate Selection

After source analysis, `CandidatePicker` filters and ranks the discovered entities.

Two selection modes are supported:

```text
kFunctionsOnly
kAll
```

In `kFunctionsOnly` mode, only entities of type `kFunction` are retained. In `kAll` mode, all supported entity types may participate.

### 9.1 Fixed-Capacity Priority Queue

CppDefense does not sort every discovered entity. Instead, it keeps only the requested top-N candidates using `FixedPriorityQueue<CodeEntityInfo, CandidatePriorityCompare>`.

Candidate strength is measured by `body_line_count()`.

For `M` discovered entities and a requested candidate count `N`, the selection stage has approximately:

```text
Time:   O(M log N)
Memory: O(N)
```

The queue is implemented using standard heap algorithms over an internal `std::vector`. The weakest retained candidate remains at the heap root, allowing a stronger incoming candidate to replace it immediately.

### 9.2 Random Target Selection

Once the top-N candidates are retained, `CandidatePicker` chooses one index using:

```text
std::mt19937
std::uniform_int_distribution
```

The default constructor seeds the generator from `std::random_device`. A deterministic seed can be supplied for tests.

## 10. Result File Generation

After an entity is selected, `ResultFile::Create()` creates:

```text
cache/current/result.txt
```

The file contains the original entity signature and an empty body.

Example source:

```cpp
int Sum(int a, int b) {
  return a + b;
}
```

Generated result template:

```cpp
int Sum(int a, int b) {

}
```

For classes, structs, and scoped enums, the suffix following the closing brace is preserved. This is required to retain syntax such as the trailing semicolon:

```cpp
struct Config {

};
```

The user edits only `result.txt` during the defense.

## 11. Cached Source Masking

`FileMasker` removes the selected entity implementation from the cached project.

The implementation is not physically shortened. Instead, characters inside the entity body are replaced with spaces while newline characters are preserved.

Example:

```cpp
int Sum(int a, int b) {
  return a + b;
}
```

becomes conceptually:

```cpp
int Sum(int a, int b) {
               
}
```

This design preserves the following invariants:

```text
file byte size remains unchanged
line endings remain unchanged
line numbers remain stable
subsequent byte offsets remain stable
opening and closing body braces remain present
```

The stored `CodeEntityInfo` offsets can therefore continue to identify the same region after masking.

## 12. Defense Timer

`DefenseTimer` uses `std::chrono::steady_clock`.

A monotonic clock is used because defense duration must not depend on wall-clock changes.

At start time:

```text
deadline = current steady time + configured duration
```

The timer provides:

```text
Start()
Stop()
running()
expired()
remaining()
```

The v1 CLI checks expiration when commands are processed. It does not use a background thread to interrupt terminal input.

## 13. Check Workflow

`DefenseSession::Check()` validates the current contents of `result.txt`.

The check pipeline is:

```text
verify active session and timer
        ↓
read result.txt
        ↓
create temporary check workspace
        ↓
patch submitted entity
        ↓
configure project
        ↓
build project
        ↓
run tests
        ↓
update session state and report
```

## 14. Temporary Check Workspace

`CheckWorkspace` creates an isolated temporary directory under:

```text
cache/current/check/
```

The masked cached project is copied into that directory before every check.

The model is therefore:

```text
original project
      │
      └─ never modified

cached project
      │
      └─ permanent masked copy for the active session

result.txt
      │
      └─ user submission

check project
      │
      └─ temporary copy containing the submitted implementation
```

This separation is a central system invariant. User-written C++ code is never inserted into the permanent masked cache.

### 14.1 RAII Cleanup

`DefenseSession` uses a local `CheckWorkspaceGuard` to clean the temporary check workspace.

The guard owns cleanup responsibility for the duration of `Check()` and calls `CheckWorkspace::Cleanup()` in its destructor.

This ensures that cleanup is performed for normal returns and early error returns without requiring repeated manual cleanup calls.

## 15. Solution Patching

`FilePatcher` inserts the submitted code into the temporary check project.

The component first computes the source file path relative to the permanent cached project. It then maps that relative path into the temporary check project.

The selected source range is replaced using:

```text
[start_offset, end_offset)
```

with the complete contents of `result.txt`.

Because patching is performed only in the temporary check copy, the masked project remains unchanged after every attempt.

## 16. Build and Test Execution

`BuildRunner` currently supports CMake projects.

A check consists of three independent build stages.

### 16.1 Configure

```bash
cmake -S <project> -B <build> -DCMAKE_BUILD_TYPE=Release
```

### 16.2 Build

```bash
cmake --build <build> --config Release
```

### 16.3 Tests

```bash
ctest --test-dir <build> --build-config Release --output-on-failure
```

Each stage produces a `BuildStepResult` containing:

```text
whether the step was attempted
whether it succeeded
exit code
captured output
```

`BuildResult` contains separate configure, build, and test results.

If configure fails, build and tests are skipped. If build fails, tests are skipped. A defense attempt is successful only when all three stages succeed.

### 16.4 Log Capture

The current dependency-free process implementation uses `std::system` and redirects standard output and standard error into files.

Logs are stored under:

```text
cache/current/logs/configure.log
cache/current/logs/build.log
cache/current/logs/tests.log
```

The files are then read back into `BuildStepResult::output` for CLI reporting and final result generation.

## 17. Retry Semantics

A failed configure, build, or test stage does not automatically terminate the defense.

The behavior is:

```text
Check attempt
   ↓
configure/build/tests failed
   ↓
attempt counter incremented
   ↓
status returns to Active
   ↓
user edits result.txt
   ↓
new Check attempt
```

This cycle may continue until either:

- the submitted implementation passes configure, build, and tests; or
- the defense timer expires.

## 18. Timeout Semantics

Expiration is checked before a new check begins and again after build execution.

The second check is important because a configure/build/test cycle may itself consume the remaining defense time.

If the timer expires during build execution, the session becomes `kExpired` even if the resulting code eventually builds successfully.

## 19. Final Defense Result

When a defense reaches a terminal state, `DefenseResultWriter` creates:

```text
cache/current/defense_result.txt
```

`DefenseResult` contains:

```text
selected entity
final status
attempt count
elapsed time
last build/test log
```

Typical terminal states are:

```text
kSuccess
kFailed
kExpired
kError
```

If the user explicitly finishes an active defense before success, the session is recorded as failed.

## 20. Runtime Workspace Layout

For a project named `labwork_simple`, the active session is organized as follows:

```text
cache/current/
├── project/
│   └── labwork_simple/       # persistent masked copy
├── logs/
│   ├── configure.log
│   ├── build.log
│   └── tests.log
├── metadata/
├── result.txt                # user-edited submission
└── defense_result.txt        # final session report
```

During a check, the following temporary structure is added:

```text
cache/current/check/
├── project/
│   └── labwork_simple/       # patched temporary copy
└── build/                    # temporary CMake build directory
```

The entire `check/` directory is removed after the attempt.

## 21. Core Invariants

The v1 architecture is based on several invariants that should be preserved by future changes.

### 21.1 Original-project isolation

The original user project must never be modified by defense operations.

### 21.2 Masked-cache isolation

The permanent cached project must remain masked after `Start()`. User submissions must not be written into this copy.

### 21.3 Submission isolation

`result.txt` is the user-controlled source of the restored entity during an active defense.

### 21.4 Temporary validation

Submitted code is compiled and tested only after being inserted into a disposable check workspace.

### 21.5 Stable parser coordinates

Masking must preserve byte size and line endings so that previously calculated entity offsets remain valid.

### 21.6 Explicit error propagation

Recoverable operations return typed `std::expected<Result, Error>` values rather than relying on implicit failure state.

## 22. C++ Concepts Demonstrated by the Implementation

The project intentionally applies several modern C++ concepts in production-style code.

### 22.1 RAII

RAII is used by standard stream objects and by `CheckWorkspaceGuard`, which owns cleanup of the temporary validation workspace.

### 22.2 Move Semantics

Paths, error values, prepared projects, and candidate queues are moved where ownership transfer is appropriate.

### 22.3 `std::expected`

Filesystem, parsing, selection, build, and session operations use explicit success-or-error contracts.

### 22.4 `std::optional`

`DefenseSession` uses optional values for state that does not exist before session initialization, including the selected entity and workspace.

### 22.5 Templates and Generic Algorithms

`FixedPriorityQueue<T, Compare>` demonstrates a class template with a configurable comparator and uses `std::push_heap` and `std::pop_heap` internally.

### 22.6 Random Number Generation

Candidate selection uses `std::mt19937` and `std::uniform_int_distribution`, with deterministic seeding available for tests.

### 22.7 `std::filesystem`

Workspace preparation, source discovery, temporary-copy creation, path mapping, and cleanup are implemented using `std::filesystem`.

### 22.8 `std::chrono`

Defense timing is implemented with `std::chrono::steady_clock` and explicit duration types.

### 22.9 Composition

`DefenseSession` coordinates specialized components rather than inheriting from them. This keeps responsibilities narrow and allows infrastructure implementations to evolve independently.

## 23. Extensibility

The current architecture provides clear extension points.

Potential future changes include:

- additional build-system implementations behind the build execution boundary;
- a richer parser or AST-based source analyzer;
- session persistence across application restarts;
- a web or graphical user interface reusing the application layer;
- richer terminal progress and countdown display;
- configurable build and test commands;
- session history and statistics.

The principal requirement for future extensions is that the core defense rules remain independent of the user-interface implementation and that the original-project and masked-cache isolation guarantees remain intact.

## 24. End-to-End Execution Summary

The complete v1 flow can be summarized as follows.

### Start

```text
selected project
      ↓
WorkspaceCache
      ↓
ProjectScanner
      ↓
SourceFileRepository
      ↓
SimpleSourceParser
      ↓
CodeEntityInfo[]
      ↓
CandidatePicker
      ↓
selected entity
      ├─ ResultFile → result.txt
      └─ FileMasker → masked cached project
      ↓
DefenseTimer starts
```

### Defense

```text
user edits result.txt
        +
DefenseTimer remains active
```

### Check

```text
result.txt
    ↓
CheckWorkspace
    ↓
FilePatcher
    ↓
BuildRunner
    ├─ configure
    ├─ build
    └─ CTest
    ↓
pass / retry / timeout
    ↓
DefenseResultWriter
```

The defining operational rule of CppDefense v1 is therefore:

```text
original project  → never modified
cached project    → permanently masked during the session
result.txt        → user submission
check project     → disposable validation copy
```
