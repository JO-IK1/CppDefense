cmake_minimum_required(VERSION 3.20)

project(CppDefense VERSION 0.5.0 LANGUAGES CXX)

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
  src/application/function_picker.cpp
  src/application/defense_timer.cpp

  src/infrastructure/workspace_cache.cpp
  src/infrastructure/project_scanner.cpp
  src/infrastructure/source_file_repository.cpp
  src/infrastructure/simple_source_parser.cpp
  src/infrastructure/file_masker.cpp
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

if (MSVC)
  target_compile_options(cpp-defense-tests PRIVATE /W4)
  target_compile_options(cpp-defense-project-scanner-tests PRIVATE /W4)
  target_compile_options(cpp-defense-source-file-repository-tests PRIVATE /W4)
  target_compile_options(cpp-defense-source-parser-tests PRIVATE /W4)
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
  enum-class-ignored
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
