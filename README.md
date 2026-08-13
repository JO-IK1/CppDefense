# CppDefense

CppDefense is an educational C++ project for practicing lab defense tasks.

The idea is to simulate a programming defense session: the application analyzes a C++ project, selects candidate functions, hides one of them, starts a timer, and asks the user to restore the missing implementation.

## Current Status

Version: `v0.4.1`

The application provides an interactive CLI for configuring and running a defense session.

Currently implemented:

- startup command-line argument parsing;
- startup without a project path;
- interactive command parsing;
- project directory selection and validation;
- configurable candidate function count;
- configurable timer duration;
- selectable processing mode;
- commands for starting and checking a defense session;
- defense session status model;
- typed cache and filesystem errors;
- workspace model for source, cache, build, metadata, log, and result paths;
- absolute and canonical project path resolution;
- calculation of the runtime cache layout inside the CppDefense project;
- validation of dangerous source/cache path relationships;
- safe cleanup and creation of the runtime workspace;
- recursive project copying with symbolic-link rejection;
- workspace preparation through the interactive `--start` command;
- recursive source-file discovery with deterministic ordering;
- filtering of unsupported files and generated or editor directories;
- symbolic-link rejection for scan roots and symbolic-link skipping during
  traversal;
- automated workspace cache and project scanner tests;
- separation between UI, application, core, and infrastructure layers.

The `--start` command now prepares an isolated project copy. Project scanning
is implemented as a separate infrastructure module and will be connected to
the defense-session pipeline next. The `--check` command still provides
placeholder behavior. Source-code modification and restoration checking are
still under development.

The runtime workspace is located under `cache/current`. Preparing a new
session safely removes the previous `current` workspace, creates the required
directories, and copies the selected project. The root `cache` directory is
excluded from version control.

## Project Structure

```text
CppDefense/
├── apps
│   └── cli
│       └── main.cpp
├── include
│   └── cpp_defense
│       ├── application/
│       ├── core/
│       ├── infrastructure/
│       └── ui/
├── src
│   ├── application/
│   ├── infrastructure/
│   └── ui/
├── tests
├── CMakeLists.txt
└── README.md
```

The project uses a layered structure:

- `core/` — pure business logic;
- `application/` — application use cases;
- `infrastructure/` — filesystem, external tools, adapters;
- `ui/` — console interface and user interaction.

## Build

Requirements:

- C++23 compiler;
- CMake 3.20+.

Build the project:

```bash
cmake -S . -B build
cmake --build build
```

Run the tests:

```bash
ctest --test-dir build --output-on-failure
```

CTest reports the workspace cache and project scanner scenarios separately, so
a failed case can be identified by name.

## CI/CD

GitHub Actions builds and tests the project on Linux, macOS, and Windows for
every push and pull request. Pushing a tag matching `v*` packages all three CLI
binaries and publishes them in a GitHub Release after every platform succeeds.
The packaged layout keeps the executable under `bin` and places its runtime
cache in the extracted package root.

Run the application:

```bash
./build/cpp-defense --help
```

Example:

```bash
./build/cpp-defense ./some_project --functions 5 --timer 10
```

## CLI Usage

The project directory is optional:

```bash
cpp-defense [project_path] [options]
```

Startup options:

```text
-h, --help                 Show help and exit
-p, --path <directory>     Select a project directory
-n, --functions <count>    Set candidate count from 1 to 50
-t, --timer <minutes>      Set timer duration from 1 to 180 minutes
    --functions-only       Select functions only (default)
    --all                  Allow all supported code fragments
```

Example:

```bash
cpp-defense
cpp-defense ./lab_work
cpp-defense -p ./lab_work -n 10 -t 15 --functions-only
```

## Interactive Mode

After startup, the application remains open and waits for commands. If no project directory was provided at startup, it can be selected interactively.

Available commands:

```text
-h, --help                 Show help
-p, --path <directory>     Select or change the project directory
-n, --functions <count>    Change candidate count from 1 to 50
-t, --timer <minutes>      Change timer duration from 1 to 180 minutes
    --functions-only       Switch to functions-only mode
    --all                  Allow all supported code fragments
-s, --start                Start the defense session
-c, --check                Check the restored function
-e, --exit                 Close the application
```

Example:

```text
CppDefense CLI
Version: 0.4.1
Project path: not selected
Function candidates: 5
Timer: 5 minutes
Mode: functions only

> -n 10
Function candidates: 10
> -t 15
Timer: 15 minutes
> -p ./lab_work
Project path selected: "./lab_work"
> -s
Starting defense...
> -e
```

A project directory must be selected before running -s or --start.
The project path is optional at startup. After startup, the application waits for commands; use `-p <directory>` to select a project and `-e` or `--exit` to close it. The function count, timer, and mode can also be changed interactively.

## Recent Changes

### Project source discovery

- Added recursive discovery of C and C++ source and header files.
- Added case-insensitive source-extension matching and deterministic result
  ordering.
- Added exclusion of build, cache, version-control, IDE, and editor
  directories.
- Added symbolic-link skipping during traversal and rejection of symbolic-link
  scan roots, including dangling links.
- Added typed project-scanning errors with filesystem context.
- Split cache and scanner errors into module-specific headers.
- Added automated tests for discovery, filtering, traversal, and invalid scan
  roots.

### Runtime workspace preparation

- Added safe cache cleanup restricted to `cache/current`.
- Added rejection of overlapping source and cache paths.
- Added symbolic-link validation for the source tree and cleanup paths.
- Added runtime directory creation and recursive source project copying.
- Connected workspace preparation to the interactive `--start` command.
- Added automated tests for successful preparation and destructive edge cases.
- Replaced the duplicated CLI version string with the CMake project version.

### Workspace path model and cache preparation foundation

- Added the `Workspace` model for all defense session paths.
- Added defense session status values.
- Added typed cache and filesystem errors.
- Added `WorkspaceCache` path calculation.
- Added validation and canonicalization of the CppDefense and source project
  directories.
- Added calculation of project, build, metadata, log, and result paths under
  `cache/current`.
- Added dedicated errors for a missing CppDefense root and an invalid source
  project name.
- Excluded the runtime cache from version control.
- Updated the project version to `v0.3.1`.

### Unified startup and interactive command parsing

- Added startup without a required project path.
- Added interactive parsing through `CommandParser`.
- Added interactive configuration of the project path, function count,
  timer, and processing mode.
- Reused the same validation rules for startup and interactive commands.
- Added structured interactive command results.
- Simplified `CliApp` by moving parsing and validation into `CommandParser`.
- Added `--start`, `--check`, `--help`, and `--exit` command handling.
- Updated CLI usage output and project documentation.
