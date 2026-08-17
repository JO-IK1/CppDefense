# CppDefense Learning Map

CppDefense is intentionally structured as a practical review of university C++
topics rather than only as a utility.

## Language and project fundamentals

- **Compilation / CMake** — root CMake project, example project and BuildRunner.
- **Namespaces** — all application code lives under `cpp_defense`.
- **Strings and byte offsets** — source repository, parser, masker and patcher.
- **References / const-correctness** — component interfaces pass large values by
  `const&` and expose read-only access where mutation would violate invariants.

## STL and algorithms

- **`std::vector`** — source files, parser results and internal heap storage.
- **Algorithms / iterators** — parser helpers and heap algorithms.
- **`std::filesystem`** — WorkspaceCache, ProjectScanner and check workspace.
- **`std::optional`** — current session/entity state.
- **`std::expected`** — explicit error-return contracts throughout v1.
- **`std::variant`** — preparation-error composition.

## Resource management

- **RAII** — streams, temporary check-workspace cleanup guard and object-owned
  session state.
- **Move semantics** — paths, errors, prepared projects and candidate queues are
  moved where ownership is transferred.
- **Rule of zero / special members** — value-oriented components avoid manual
  memory management; non-copyable UI objects make ownership explicit.

## Templates

- **Class templates** — `FixedPriorityQueue<T, Compare>`.
- **Function objects / comparators** — candidate priority ordering.
- **Generic standard algorithms** — `std::push_heap` / `std::pop_heap`.

## Randomness

- **`std::mt19937`** — random defense target selection.
- **`std::uniform_int_distribution`** — unbiased queue-index selection.
- deterministic seeds are supported for tests.

## Time and state

- **`std::chrono::steady_clock`** — monotonic defense timer.
- **State machines / enums** — `DefenseStatus` tracks idle, active, checking,
  success, failure and expiration.

## Files and processes

- **`std::ifstream` / `std::ofstream`** — byte-exact source/result/log handling.
- **`std::system`** — dependency-free first implementation of CMake/CTest
  process execution.
- **Exit codes** — configure, build and test outcomes are stored independently.

## Architecture and OOP

- **Composition** — `DefenseSession` coordinates small components rather than
  inheriting from infrastructure classes.
- **Encapsulation** — heap invariants and session state are hidden behind narrow
  APIs.
- **Layered design** — core/application/infrastructure/ui dependencies keep the
  console separate from reusable defense logic.

## Testing

- deterministic filesystem fixtures;
- malformed input tests;
- RNG seed tests;
- fake-clock timer tests;
- real nested CMake/CTest integration tests;
- end-to-end failed-attempt → retry → success tests.
