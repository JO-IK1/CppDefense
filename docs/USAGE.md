# Using CppDefense

This guide covers installation, CLI commands and the complete defense workflow.
For implementation details, see [ARCHITECTURE.md](ARCHITECTURE.md).

## Requirements

- a C++23 compiler for building CppDefense;
- CMake 3.20 or newer;
- CTest, which is included with CMake;
- a trusted CMake-based C or C++ project with tests.

CppDefense configures, builds and executes the selected project's tests. Do not
use it with untrusted projects.

## Build

From the repository root:

```bash
cmake -S . -B build
cmake --build build
ctest --test-dir build --output-on-failure
```

The executable is created at `build/cpp-defense` on single-config generators.
Multi-config generators may place it under `build/Debug` or `build/Release`.

To create a platform archive containing the executable, license and docs:

```bash
cmake --build build --target package
```

## Launch

Start without a selected project:

```bash
./build/cpp-defense
```

Or provide the project and initial settings immediately:

```bash
./build/cpp-defense ./examples/labwork_simple -n 5 -t 10
```

Startup options:

```text
-p, --path <directory>     Select a project directory
-n, --functions <count>    Candidate count from 1 to 50
-t, --timer <minutes>      Timer from 1 to 180 minutes
    --functions-only       Select only functions (default)
    --all                  Allow every supported entity type
-h, --help                 Show help
```

The project may also be passed as the first positional argument.

## Interactive commands

```text
start, -s, --start         Start or restart a defense session
check, build, -c, --check  Check the current result.txt
info, i                    Show the selected entity and paths
time                       Show remaining time
-p, --path <directory>     Change the selected project
-n, --functions <count>    Change the candidate count
-t, --timer <minutes>      Change the timer
    --functions-only       Select only functions
    --all                  Allow all supported entities
help, -h, --help           Show help
quit, q, -e, --exit        Finish the session and exit
```

## Start a defense

After choosing a project, run:

```text
> start
```

CppDefense then:

1. creates an isolated session workspace;
2. copies the project without modifying the original;
3. discovers and parses supported source files;
4. retains the requested number of largest candidates;
5. randomly selects one entity;
6. masks its implementation in the cached copy;
7. creates `result.txt` and starts the timer.

The CLI prints the selected entity, cached project path, editable result path,
final report path and remaining time.

## Restore the body

`result.txt` contains instructions similar to:

```cpp
// Restore only the body contents for CalculateStatistics.
// The declaration and outer braces are preserved by CppDefense.
```

Replace those lines with only the contents that belong between the original
outer braces. For a function, an answer might be:

```cpp
return first + second;
```

Do not repeat the function signature or add the outer `{}`. CppDefense checks
the fragment's brace and parenthesis structure before applying it.

The masked cached source can be inspected when the original declaration is
needed. Its location is shown by `info`.

## Check an answer

Run:

```text
> check
```

The application creates a temporary copy, inserts the submitted body and runs:

```text
CMake configure → build → CTest
```

Possible outcomes:

- configure, build or tests fail — logs are printed and the session remains
  active for another attempt;
- every stage succeeds — the defense finishes with `success`;
- the deadline is reached — the running process tree is terminated and the
  defense finishes with `expired`;
- the user exits before success — the defense finishes with `failed`.

Closing standard input with EOF also finalizes and saves an active session.

## Runtime workspace

The default runtime root is:

- Windows: `%LOCALAPPDATA%/CppDefense`;
- macOS: `~/Library/Caches/CppDefense`;
- other Unix systems: `$XDG_CACHE_HOME/cpp-defense`, falling back to
  `~/.cache/cpp-defense`.

Set `CPP_DEFENSE_HOME` to use an explicit directory.

For a project named `labwork_simple`, the active session looks like:

```text
cache/<session-id>/
├── project/
│   └── labwork_simple/       # masked session copy
├── logs/
│   ├── configure.log
│   ├── build.log
│   └── tests.log
├── metadata/
├── result.txt                # editable body
└── defense_result.txt        # final report
```

During `check`, `cache/<session-id>/check/` is created and removed automatically.

## Result report

`defense_result.txt` contains:

- the selected entity and source file;
- attempt count;
- elapsed defense time;
- final status;
- the most recent configure/build/test output.

Elapsed time is frozen when the session reaches a final state, so reopening or
resaving the report does not change the recorded duration.

## Example project

The repository includes `examples/labwork_simple`, a standalone CMake project
intended for manual smoke testing:

```bash
./build/cpp-defense ./examples/labwork_simple -n 5 -t 10
```
