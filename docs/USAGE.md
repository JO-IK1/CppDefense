# Local CLI usage

CppDefense turns a trusted CMake project into a timed code-restoration exercise.
It builds and runs the selected project's code, so do not use the local CLI with
untrusted projects.

## Requirements and build

- C++23 compiler
- CMake 3.20 or newer, including CTest
- a CMake-based C or C++ project with tests

```sh
cmake -S . -B build
cmake --build build
ctest --test-dir build --output-on-failure
```

Single-config generators create `build/cpp-defense`; multi-config generators
may place it under `build/Debug` or `build/Release`.

## Start

```sh
./build/cpp-defense ./examples/labwork_simple -n 5 -t 10
```

Options:

```text
-p, --path <directory>     project directory
-n, --functions <count>    candidate count (1–50)
-t, --timer <minutes>      time limit (1–180)
    --functions-only       select functions only (default)
    --all                  allow every supported entity type
-h, --help                 show help
```

The project path may be the first positional argument.

## Defense workflow

At the interactive prompt, use `start` to create a session. CppDefense copies
the project, selects an entity, masks its implementation, creates `result.txt`,
and starts the timer.

Edit `result.txt` with only the contents that belong inside the original outer
braces. Do not repeat the declaration or the braces. For example:

```cpp
return first + second;
```

Use `check` to create a temporary project, insert the answer, configure and
build it, and run CTest. A failed attempt leaves the session active until the
deadline. A successful test run completes it. `quit` or end-of-file finalizes
an unfinished session as failed.

Useful commands:

```text
start                    start or restart the defense
check                    validate result.txt
info                     show the entity and workspace paths
time                     show remaining time
help                     show all commands
quit                     save and exit
```

## Workspace and results

The runtime root is `%LOCALAPPDATA%/CppDefense` on Windows,
`~/Library/Caches/CppDefense` on macOS, and `$XDG_CACHE_HOME/cpp-defense` (or
`~/.cache/cpp-defense`) on other Unix systems. Override it with
`CPP_DEFENSE_HOME`.

Each session has this layout:

```text
cache/<session-id>/
├── project/              masked project copy
├── logs/                 configure, build, and test logs
├── metadata/
├── result.txt            editable answer
└── defense_result.txt    final status and report
```

The temporary `check/` directory is recreated for each attempt. The final report
records the selected entity, attempts, elapsed time, status, and latest bounded
logs. The original project is never modified.

For implementation and isolation details, see [Architecture](ARCHITECTURE.md).
