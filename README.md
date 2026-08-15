# CppDefense

CppDefense is a C++ training tool that simulates a programming lab defense.

During a defense session, the application creates an isolated copy of a C++
project, analyzes its source code, selects a code entity, hides its
implementation, and asks the user to restore it within a limited amount of
time.

The project is inspired by university C++ lab defenses where a student has to
restore a removed part of their own code and prove that the project still
builds and works correctly.

## Current Status

Version: `v0.5.0`

CppDefense is currently under active development.

The project can already:

- configure a defense session through an interactive CLI;
- create an isolated workspace for a selected project;
- safely validate and copy a source project;
- recursively discover C and C++ source files;
- exclude build, cache, test, IDE, and version-control directories;
- reject unsafe symbolic-link configurations;
- read source files without modifying their original byte representation;
- lexically analyze C++ source code;
- ignore comments, string literals, character literals, raw strings, and
  preprocessor directives during structural analysis;
- validate matching parentheses and braces;
- discover supported source-code entities;
- preserve exact source lines and byte offsets for discovered entities;
- report filesystem, scanner, source-reading, and parser failures through
  typed errors.

Currently supported code entities are:

- functions;
- classes;
- structures.

The candidate selection, source masking, build execution, timer integration,
and final restoration check are the next development stages.

## Example

Given the following source:

```cpp
struct Config {
  int width;
  int height;
};

class Player {
 public:
  void Move() {
  }
};

int Sum(int a, int b) {
  return a + b;
}
```

CppDefense can currently discover:

```text
[struct] Config
[class] Player
[function] Move
[function] Sum
```

Each discovered entity stores its source file, line range, full entity range,
and body range.

This information will later be used to safely remove or mask an entity during
a defense session.

## Processing Pipeline

```text
Selected project
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

## Architecture

CppDefense uses a layered architecture:

```text
core/
application/
infrastructure/
ui/
```

### `core`

Contains shared domain models and typed errors.

Examples:

```text
CodeEntityInfo
Workspace
DefenseStatus
CacheError
ScanError
ParseError
SourceFileError
```

### `application`

Coordinates application use cases.

Current components include:

```text
DefenseService
DefenseSession
DefenseTimer
CandidatePicker
```

### `infrastructure`

Contains filesystem and external-process related implementations.

Current components include:

```text
WorkspaceCache
ProjectScanner
SourceFileRepository
SimpleSourceParser
FileMasker
BuildRunner
```

### `ui`

Contains console interaction and command parsing.

```text
CliApp
CommandParser
```

The CLI should not contain the core defense logic. Temporary Phase 2 and
Phase 3 diagnostic output is currently present while the analysis pipeline is
being developed.

## Project Structure

```text
CppDefense/
├── apps/
│   └── cli/
│       └── main.cpp
│
├── include/
│   └── cpp_defense/
│       ├── application/
│       ├── core/
│       ├── infrastructure/
│       └── ui/
│
├── src/
│   ├── application/
│   ├── infrastructure/
│   └── ui/
│
├── tests/
│   ├── workspace_cache_test.cpp
│   ├── project_scanner_test.cpp
│   ├── source_file_repository_test.cpp
│   └── simple_source_parser_test.cpp
│
├── .github/
│   └── workflows/
│       └── ci-cd.yml
│
├── CMakeLists.txt
└── README.md
```

## Requirements

- C++23 compiler;
- CMake 3.20+.

The project currently uses C++23 primarily for `std::expected`.

## Build

Configure:

```bash
cmake -S . -B build
```

Build:

```bash
cmake --build build
```

Run all tests:

```bash
ctest --test-dir build --output-on-failure
```

At the current development stage, the test suite contains 46 individual
CTest scenarios.

## Run
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
-c, --check                Check the restored code entity
-e, --exit                 Close the application
```

Example:

```text
CppDefense CLI
Version: 0.5.0
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

The current `--start` implementation prepares the isolated workspace, scans
the source project, and prints discovered source entities for development
verification.

The complete defense-session workflow is not implemented yet.

## CI/CD

GitHub Actions builds and tests CppDefense on:

- Linux;
- macOS;
- Windows.

Every push and pull request runs the build and test pipeline.

Version tags matching `v*` can be packaged into platform-specific release
artifacts.

## Testing

Tests currently cover:

### Workspace cache

- fresh workspace creation;
- replacement of an existing workspace;
- source/cache path intersection;
- symbolic-link rejection;
- safe cleanup.

### Project scanner

- supported source extensions;
- deterministic source discovery;
- directory exclusion;
- case-insensitive exclusions;
- custom exclusions;
- absolute normalized paths;
- symbolic-link skipping;
- invalid scan roots.

### Source file repository

- normal file reading;
- empty files;
- CRLF preservation;
- missing files.

### Source parser

- simple functions;
- classes;
- structures;
- inline methods;
- inheritance;
- nested classes;
- mixed entity types;
- qualified methods;
- multiline signatures;
- ignored namespaces;
- ignored enum classes;
- ignored forward declarations;
- ignored control statements;
- ignored lambdas;
- comments and literals;
- exact entity byte ranges;
- exact body byte ranges;
- CRLF offsets;
- malformed comments and strings;
- unmatched braces.

## Roadmap

### Completed

- [x] Interactive CLI foundation
- [x] Typed error model
- [x] Isolated workspace preparation
- [x] Safe project copying
- [x] Recursive C/C++ source discovery
- [x] Exact source-file reading
- [x] Lexical source sanitization
- [x] Structural bracket analysis
- [x] Function discovery
- [x] Class discovery
- [x] Structure discovery
- [x] Exact source ranges for discovered entities
- [x] Automated tests for the analysis pipeline
- [x] Multi-platform CI

### Next

- [ ] Move source-analysis orchestration from temporary CLI debug code into
      the application layer
- [ ] Filter entities according to `--functions-only` and `--all`
- [ ] Rank/select defense candidates
- [ ] Randomly choose a defense target
- [ ] Mask the selected entity in the isolated workspace
- [ ] Implement project build execution
- [ ] Capture compiler output
- [ ] Connect the defense timer
- [ ] Implement `--check`
- [ ] Restore or finish a defense session cleanly

### Later

- [ ] Detect project build systems
- [ ] Improve C++ syntax coverage
- [ ] Persist defense metadata
- [ ] Add session statistics
- [ ] Improve terminal UX
- [ ] Prepare a polished portfolio release
