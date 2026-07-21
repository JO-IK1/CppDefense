# CppDefense

CppDefense is an educational C++ project for practicing lab defense tasks.

The idea is to simulate a programming defense session: the application analyzes a C++ project, selects candidate functions, hides one of them, starts a timer, and asks the user to restore the missing implementation.

## Current Status

Version: `v0.3.0`

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
- separation between UI, application, core, and infrastructure layers.

The `--start` and `--check` commands currently provide placeholder behavior.
Full source-code modification and restoration checking are still under development.

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
Version: 0.3.0
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
