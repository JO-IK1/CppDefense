# CppDefense Roadmap

## v1.0 — console application

Completed:

- isolated project workspace;
- recursive C/C++ scanner;
- lightweight lexical/structural parser;
- functions, methods, classes, structs and `enum class`;
- top-N candidate selection;
- random target selection;
- body masking and `result.txt` generation;
- timer and retryable defense session;
- temporary check project;
- solution patching;
- CMake configure/build and CTest execution;
- compiler/test log capture;
- final defense report;
- example lab project;
- multi-platform CI;
- component and end-to-end tests.

The original source project is never modified. The earlier FileBackup idea was
superseded by the isolated-copy model.

## Possible v1.x improvements

- detect Ninja/Make/MSBuild or user-provided build commands;
- persist/recover an active session after application restart;
- richer terminal UI and visible countdown;
- candidate preview/roulette UI;
- configurable excluded directories;
- configurable test command;
- session history and statistics;
- more parser syntax coverage.

## Possible v2

After the console version is stable, the application layer can be reused by a
web/API frontend. Core selection and defense rules should remain independent
from the UI.
