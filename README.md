# CppDefense

CppDefense is an educational C++ project for practicing lab defense tasks.

The idea is to simulate a programming defense session: the application analyzes a C++ project, selects candidate functions, hides one of them, starts a timer, and asks the user to restore the missing implementation.

The project is developed step by step: from a minimal console version to a more complete portfolio-ready application.

## Current Status

Version: `v0.2`

At the current stage, the project contains the basic CLI application structure:

- command-line argument parsing;
- CLI options model;
- basic application runner;
- help output;
- validation of the project path;
- configurable number of functions;
- configurable timer duration;
- early separation between UI, application logic, and future core logic.

The project does not yet analyze C++ source files or hide functions. These features will be added in later versions.

## Project Structure

```text
CppDefense/
├── CMakeLists.txt
├── README.md
├── src/
│   └── main.cpp
├── include/
│   └── cpp_defense/
│       ├── application/
│       ├── core/
│       ├── infrastructure/
│       └── ui/
└── tests/
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

Run the application:

```bash
./build/cpp-defense --help
```

Example:

```bash
./build/cpp-defense ./some_project --functions 5 --timer 10
```

## CLI Usage

```bash
cpp-defense <project_path> [options]
```

Options:

```text
-h, --help              Show help message
-n, --functions <num>   Number of functions to select
-t, --timer <minutes>   Timer duration in minutes
--functions-only        Work only with functions
--all                   Use all supported code entities
```

Example:

```bash
cpp-defense ./lab_work --functions 5 --timer 10
```

## Recent Changes

### Refactor CLI parsing and app structure

- Moved CLI logic from headers to source files.
- Added structured command parsing.
- Added CLI option validation.
- Added help handling.
- Added explicit application exit codes.
- Cleaned up `main.cpp`.
- Improved names and defaults in CLI options.