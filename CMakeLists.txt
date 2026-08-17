cmake_minimum_required(VERSION 3.20)

project(CppDefense VERSION 1.0.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 23)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)
set(CMAKE_EXPORT_COMPILE_COMMANDS ON)

# =========================
# Library with project logic
# =========================

add_library(cpp-defense-lib
  src/application/defense_service.cpp
  src/application/defense_session.cpp
  src/application/candidate_picker.cpp
  src/application/defense_timer.cpp

  src/infrastructure/workspace_cache.cpp
  src/infrastructure/project_scanner.cpp
  src/infrastructure/source_file_repository.cpp
  src/infrastructure/simple_source_parser.cpp
  src/infrastructure/source_parser/source_lexer.cpp
  src/infrastructure/source_parser/source_structure.cpp
  src/infrastructure/source_parser/source_function_parser.cpp
  src/infrastructure/source_parser/source_type_parser.cpp
  src/infrastructure/source_parser/source_entity_finder.cpp
  src/infrastructure/file_masker.cpp
  src/infrastructure/result_file.cpp
  src/infrastructure/file_patcher.cpp
  src/infrastructure/check_workspace.cpp
  src/infrastructure/defense_result_writer.cpp
  src/infrastructure/build_runner.cpp

  src/ui/cli_app.cpp
  src/ui/command_parser.cpp
)

target_include_directories(cpp-defense-lib
  PUBLIC
    ${PROJECT_SOURCE_DIR}/include
)

target_compile_features(cpp-defense-lib
  PUBLIC
    cxx_std_23
)

target_compile_definitions(cpp-defense-lib
  PRIVATE
    CPP_DEFENSE_VERSION="${PROJECT_VERSION}"
)

if (MSVC)
  target_compile_options(cpp-defense-lib PRIVATE /W4)
else()
  target_compile_options(cpp-defense-lib PRIVATE -Wall -Wextra -Wpedantic)
endif()


# =========================
# CLI executable
# =========================

add_executable(cpp-defense
  apps/cli/main.cpp
)

target_link_libraries(cpp-defense
  PRIVATE
    cpp-defense-lib
)

install(TARGETS cpp-defense
  RUNTIME DESTINATION bin
)


# =========================
# Tests
# =========================

enable_testing()

add_executable(cpp-defense-tests
  tests/workspace_cache_test.cpp
)

add_executable(cpp-defense-project-scanner-tests
  tests/project_scanner_test.cpp
)

add_executable(cpp-defense-source-file-repository-tests
  tests/source_file_repository_test.cpp
)

add_executable(cpp-defense-source-parser-tests
  tests/simple_source_parser_test.cpp
)

add_executable(cpp-defense-fixed-priority-queue-tests
  tests/fixed_priority_queue_test.cpp
)

add_executable(cpp-defense-candidate-picker-tests
  tests/candidate_picker_test.cpp
)

add_executable(cpp-defense-file-masker-tests
  tests/file_masker_test.cpp
)

add_executable(cpp-defense-result-file-tests
  tests/result_file_test.cpp
)

add_executable(cpp-defense-file-patcher-tests
  tests/file_patcher_test.cpp
)

add_executable(cpp-defense-timer-tests
  tests/defense_timer_test.cpp
)

add_executable(cpp-defense-build-runner-tests
  tests/build_runner_test.cpp
)

add_executable(cpp-defense-session-tests
  tests/defense_session_test.cpp
)

target_link_libraries(cpp-defense-tests
  PRIVATE
    cpp-defense-lib
)

target_link_libraries(cpp-defense-project-scanner-tests
  PRIVATE
    cpp-defense-lib
)

target_link_libraries(cpp-defense-source-file-repository-tests
  PRIVATE
    cpp-defense-lib
)

target_link_libraries(cpp-defense-source-parser-tests
  PRIVATE
    cpp-defense-lib
)

target_link_libraries(cpp-defense-fixed-priority-queue-tests
  PRIVATE
    cpp-defense-lib
)

target_link_libraries(cpp-defense-candidate-picker-tests
  PRIVATE
    cpp-defense-lib
)

target_link_libraries(cpp-defense-file-masker-tests
  PRIVATE
    cpp-defense-lib
)

target_link_libraries(cpp-defense-result-file-tests
  PRIVATE
    cpp-defense-lib
)

target_link_libraries(cpp-defense-file-patcher-tests PRIVATE cpp-defense-lib)
target_link_libraries(cpp-defense-timer-tests PRIVATE cpp-defense-lib)
target_link_libraries(cpp-defense-build-runner-tests PRIVATE cpp-defense-lib)
target_link_libraries(cpp-defense-session-tests PRIVATE cpp-defense-lib)

if (MSVC)
  target_compile_options(cpp-defense-tests PRIVATE /W4)
  target_compile_options(cpp-defense-project-scanner-tests PRIVATE /W4)
  target_compile_options(cpp-defense-source-file-repository-tests PRIVATE /W4)
  target_compile_options(cpp-defense-source-parser-tests PRIVATE /W4)
  target_compile_options(cpp-defense-fixed-priority-queue-tests PRIVATE /W4)
  target_compile_options(cpp-defense-candidate-picker-tests PRIVATE /W4)
  target_compile_options(cpp-defense-file-masker-tests PRIVATE /W4)
  target_compile_options(cpp-defense-result-file-tests PRIVATE /W4)
  target_compile_options(cpp-defense-file-patcher-tests PRIVATE /W4)
  target_compile_options(cpp-defense-timer-tests PRIVATE /W4)
  target_compile_options(cpp-defense-build-runner-tests PRIVATE /W4)
  target_compile_options(cpp-defense-session-tests PRIVATE /W4)
else()
  target_compile_options(
    cpp-defense-tests
    PRIVATE
      -Wall -Wextra -Wpedantic
  )

  target_compile_options(
    cpp-defense-project-scanner-tests
    PRIVATE
      -Wall -Wextra -Wpedantic
  )

  target_compile_options(
    cpp-defense-source-file-repository-tests
    PRIVATE
      -Wall -Wextra -Wpedantic
  )

  target_compile_options(
    cpp-defense-source-parser-tests
    PRIVATE
      -Wall -Wextra -Wpedantic
  )

  target_compile_options(
    cpp-defense-fixed-priority-queue-tests
    PRIVATE
      -Wall -Wextra -Wpedantic
  )

  target_compile_options(
    cpp-defense-candidate-picker-tests
    PRIVATE
      -Wall -Wextra -Wpedantic
  )

  target_compile_options(
    cpp-defense-file-masker-tests
    PRIVATE
      -Wall -Wextra -Wpedantic
  )

  target_compile_options(
    cpp-defense-result-file-tests
    PRIVATE
      -Wall -Wextra -Wpedantic
  )

  target_compile_options(cpp-defense-file-patcher-tests PRIVATE -Wall -Wextra -Wpedantic)
  target_compile_options(cpp-defense-timer-tests PRIVATE -Wall -Wextra -Wpedantic)
  target_compile_options(cpp-defense-build-runner-tests PRIVATE -Wall -Wextra -Wpedantic)
  target_compile_options(cpp-defense-session-tests PRIVATE -Wall -Wextra -Wpedantic)
endif()

set(WORKSPACE_CACHE_TEST_CASES
  prepare-existing
  prepare-fresh
  reject-source-inside-cache
  reject-cache-inside-source
  reject-source-symlink
  reject-cleanup-symlink
)

foreach(test_case IN LISTS WORKSPACE_CACHE_TEST_CASES)
  add_test(
    NAME workspace-cache.${test_case}
    COMMAND cpp-defense-tests ${test_case}
  )
endforeach()

set(PROJECT_SCANNER_TEST_CASES
  find-supported-files
  skip-excluded-directories
  skip-excluded-directories-case-insensitive
  custom-excluded-directories
  absolute-paths
  skip-symbolic-links
  empty-result
  reject-missing-root
  reject-file-root
  reject-root-symlink
  reject-dangling-root-symlink
)

foreach(test_case IN LISTS PROJECT_SCANNER_TEST_CASES)
  add_test(
    NAME project-scanner.${test_case}
    COMMAND cpp-defense-project-scanner-tests ${test_case}
  )
endforeach()

set(SOURCE_FILE_REPOSITORY_TEST_CASES
  read-file
  read-empty-file
  preserve-crlf
  reject-missing-file
)

foreach(test_case IN LISTS SOURCE_FILE_REPOSITORY_TEST_CASES)
  add_test(
    NAME source-file-repository.${test_case}
    COMMAND cpp-defense-source-file-repository-tests ${test_case}
  )
endforeach()

set(SOURCE_PARSER_TEST_CASES
  empty-source
  simple-function
  simple-class
  simple-struct
  class-with-method
  struct-with-method
  class-with-inheritance
  nested-classes
  multiple-entity-types
  namespace-ignored
  enum-class
  enum-class-underlying-type
  unscoped-enum-ignored
  forward-declarations-ignored
  qualified-method
  multiline-function
  control-statements-ignored
  lambda-ignored
  comments-and-literals-ignored
  exact-class-range
  exact-function-range
  entity-lines
  crlf-offsets
  constructor-initializer-list
  operators
  noexcept-operator
  friend-operator-free
  control-flow-inside-function
  unterminated-block-comment
  unterminated-string-literal
  unmatched-opening-brace
  unmatched-closing-brace
)

foreach(test_case IN LISTS SOURCE_PARSER_TEST_CASES)
  add_test(
    NAME source-parser.${test_case}
    COMMAND cpp-defense-source-parser-tests ${test_case}
  )
endforeach()


set(FIXED_PRIORITY_QUEUE_TEST_CASES
  keeps-largest-values
  rejects-weaker-value
  pop-removes-top
  empty-operations
  indexed-access
  zero-capacity
)

foreach(test_case IN LISTS FIXED_PRIORITY_QUEUE_TEST_CASES)
  add_test(
    NAME fixed-priority-queue.${test_case}
    COMMAND cpp-defense-fixed-priority-queue-tests ${test_case}
  )
endforeach()

set(CANDIDATE_PICKER_TEST_CASES
  functions-only
  all-entities
  keeps-largest
  fewer-than-requested
  no-suitable-candidates
  invalid-candidate-count
  fixed-seed
  selected-from-queue
)

foreach(test_case IN LISTS CANDIDATE_PICKER_TEST_CASES)
  add_test(
    NAME candidate-picker.${test_case}
    COMMAND cpp-defense-candidate-picker-tests ${test_case}
  )
endforeach()


set(FILE_MASKER_TEST_CASES
  mask-function-body
  preserve-crlf-offsets
  mask-type-body
  reject-invalid-range
  reject-brace-mismatch
)

foreach(test_case IN LISTS FILE_MASKER_TEST_CASES)
  add_test(
    NAME file-masker.${test_case}
    COMMAND cpp-defense-file-masker-tests ${test_case}
  )
endforeach()

set(RESULT_FILE_TEST_CASES
  function-signature
  multiline-signature
  type-semicolon
  reject-invalid-range
  read-missing-file
)

foreach(test_case IN LISTS RESULT_FILE_TEST_CASES)
  add_test(
    NAME result-file.${test_case}
    COMMAND cpp-defense-result-file-tests ${test_case}
  )
endforeach()

set(FILE_PATCHER_TEST_CASES patch outside-project)
foreach(test_case IN LISTS FILE_PATCHER_TEST_CASES)
  add_test(NAME file-patcher.${test_case} COMMAND cpp-defense-file-patcher-tests ${test_case})
endforeach()

set(DEFENSE_TIMER_TEST_CASES expiration stop)
foreach(test_case IN LISTS DEFENSE_TIMER_TEST_CASES)
  add_test(NAME defense-timer.${test_case} COMMAND cpp-defense-timer-tests ${test_case})
endforeach()

set(BUILD_RUNNER_TEST_CASES success compile-failure test-failure unsupported)
foreach(test_case IN LISTS BUILD_RUNNER_TEST_CASES)
  add_test(NAME build-runner.${test_case} COMMAND cpp-defense-build-runner-tests ${test_case})
endforeach()

set(DEFENSE_SESSION_TEST_CASES start retry-success expired no-active-session)
foreach(test_case IN LISTS DEFENSE_SESSION_TEST_CASES)
  add_test(NAME defense-session.${test_case} COMMAND cpp-defense-session-tests ${test_case})
endforeach()
