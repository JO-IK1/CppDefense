cmake_minimum_required(VERSION 3.20)

project(CppDefense VERSION 1.2.0 LANGUAGES CXX)

include(GNUInstallDirs)

set(CMAKE_CXX_STANDARD 23)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)
set(CMAKE_EXPORT_COMPILE_COMMANDS ON)

# =========================
# Library with project logic
# =========================


option(CPP_DEFENSE_SANITIZERS "Enable address and undefined behavior sanitizers" OFF)
if(CPP_DEFENSE_SANITIZERS AND NOT MSVC)
  add_compile_options(-fsanitize=address,undefined -fno-omit-frame-pointer)
  add_link_options(-fsanitize=address,undefined)
endif()

add_library(cpp-defense-core
  src/application/candidate_picker.cpp
  src/application/defense_timer.cpp
  src/infrastructure/simple_source_parser.cpp
  src/infrastructure/source_parser/source_analysis.cpp
  src/infrastructure/source_parser/entity_parser.cpp
  src/core/sha256.cpp
)
target_include_directories(cpp-defense-core PUBLIC ${PROJECT_SOURCE_DIR}/include)
target_compile_features(cpp-defense-core PUBLIC cxx_std_23)

add_library(cpp-defense-infrastructure
  src/application/defense_service.cpp
  src/application/defense_session.cpp

  src/infrastructure/workspace_cache.cpp
  src/infrastructure/project_scanner.cpp
  src/infrastructure/source_file_repository.cpp
  src/infrastructure/file_masker.cpp
  src/infrastructure/result_file.cpp
  src/infrastructure/file_patcher.cpp
  src/infrastructure/check_workspace.cpp
  src/infrastructure/defense_result_writer.cpp
  src/infrastructure/build_runner.cpp
  src/infrastructure/process_runner.cpp
  src/infrastructure/runtime_path.cpp

)

target_include_directories(cpp-defense-infrastructure
  PUBLIC
    ${PROJECT_SOURCE_DIR}/include
)

target_compile_features(cpp-defense-infrastructure
  PUBLIC
    cxx_std_23
)

target_compile_definitions(cpp-defense-infrastructure
  PRIVATE
    CPP_DEFENSE_VERSION="${PROJECT_VERSION}"
)

if (MSVC)
  target_compile_options(cpp-defense-infrastructure PRIVATE /W4)
else()
  target_compile_options(cpp-defense-infrastructure PRIVATE -Wall -Wextra -Wpedantic)
endif()


# =========================
# CLI executable
# =========================


target_link_libraries(cpp-defense-infrastructure PUBLIC cpp-defense-core)
add_library(cpp-defense-ui src/ui/cli_app.cpp src/ui/command_parser.cpp)
target_link_libraries(cpp-defense-ui PUBLIC cpp-defense-infrastructure)
target_compile_definitions(cpp-defense-ui PRIVATE CPP_DEFENSE_VERSION="${PROJECT_VERSION}")
# Compatibility aggregate for existing CLI tests and downstream builds.
add_library(cpp-defense-lib INTERFACE)
target_link_libraries(cpp-defense-lib INTERFACE cpp-defense-ui)

option(CPP_DEFENSE_BUILD_WORKER "Build headless JSON worker" ON)
if(CPP_DEFENSE_BUILD_WORKER)
  include(FetchContent)
  FetchContent_Declare(nlohmann_json
    URL https://github.com/nlohmann/json/releases/download/v3.12.0/json.tar.xz
    URL_HASH SHA256=42f6e95cad6ec532fd372391373363b62a14af6d771056dbfc86160e6dfff7aa
    DOWNLOAD_EXTRACT_TIMESTAMP TRUE
  )
  FetchContent_MakeAvailable(nlohmann_json)
  add_library(cpp-defense-worker-adapter
    src/worker/protocol.cpp
    src/worker/workspace.cpp
    src/worker/worker.cpp
  )
  target_link_libraries(cpp-defense-worker-adapter PUBLIC
    cpp-defense-infrastructure nlohmann_json::nlohmann_json)
  add_executable(cpp-defense-worker apps/worker/main.cpp)
  target_link_libraries(cpp-defense-worker PRIVATE cpp-defense-worker-adapter)
  install(TARGETS cpp-defense-worker RUNTIME DESTINATION ${CMAKE_INSTALL_BINDIR})
  install(FILES ${nlohmann_json_SOURCE_DIR}/LICENSE.MIT
    DESTINATION ${CMAKE_INSTALL_DOCDIR}/third_party/nlohmann_json)
endif()

add_executable(cpp-defense
  apps/cli/main.cpp
)

target_link_libraries(cpp-defense
  PRIVATE
    cpp-defense-lib
)

install(TARGETS cpp-defense
  RUNTIME DESTINATION ${CMAKE_INSTALL_BINDIR}
)

install(FILES README.md LICENSE
  DESTINATION ${CMAKE_INSTALL_DOCDIR}
)
install(DIRECTORY docs/
  DESTINATION ${CMAKE_INSTALL_DOCDIR}
)


# =========================
# Tests
# =========================

enable_testing()

function(add_cpp_defense_test_target target source)
  add_executable(${target} ${source})
  target_link_libraries(${target} PRIVATE cpp-defense-lib)
  if (MSVC)
    target_compile_options(${target} PRIVATE /W4)
  else()
    target_compile_options(${target} PRIVATE -Wall -Wextra -Wpedantic)
  endif()
endfunction()

add_cpp_defense_test_target(cpp-defense-tests tests/workspace_cache_test.cpp)
add_cpp_defense_test_target(cpp-defense-project-scanner-tests tests/project_scanner_test.cpp)
add_cpp_defense_test_target(cpp-defense-source-file-repository-tests tests/source_file_repository_test.cpp)
add_cpp_defense_test_target(cpp-defense-source-parser-tests tests/simple_source_parser_test.cpp)
add_cpp_defense_test_target(cpp-defense-fixed-priority-queue-tests tests/fixed_priority_queue_test.cpp)
add_cpp_defense_test_target(cpp-defense-candidate-picker-tests tests/candidate_picker_test.cpp)
add_cpp_defense_test_target(cpp-defense-file-masker-tests tests/file_masker_test.cpp)
add_cpp_defense_test_target(cpp-defense-result-file-tests tests/result_file_test.cpp)
add_cpp_defense_test_target(cpp-defense-file-patcher-tests tests/file_patcher_test.cpp)
add_cpp_defense_test_target(cpp-defense-timer-tests tests/defense_timer_test.cpp)
add_cpp_defense_test_target(cpp-defense-build-runner-tests tests/build_runner_test.cpp)
add_cpp_defense_test_target(cpp-defense-session-tests tests/defense_session_test.cpp)
add_cpp_defense_test_target(cpp-defense-runtime-path-tests tests/runtime_path_test.cpp)
add_cpp_defense_test_target(cpp-defense-ui-tests tests/ui_test.cpp)

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
  requires-clause
  body-validation
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
  body-template
  multiline-declaration-hidden
  type-declaration-hidden
  reject-invalid-range
  read-missing-file
)

foreach(test_case IN LISTS RESULT_FILE_TEST_CASES)
  add_test(
    NAME result-file.${test_case}
    COMMAND cpp-defense-result-file-tests ${test_case}
  )
endforeach()

set(FILE_PATCHER_TEST_CASES patch reject-body-escape outside-project)
foreach(test_case IN LISTS FILE_PATCHER_TEST_CASES)
  add_test(NAME file-patcher.${test_case} COMMAND cpp-defense-file-patcher-tests ${test_case})
endforeach()

set(DEFENSE_TIMER_TEST_CASES expiration stop)
foreach(test_case IN LISTS DEFENSE_TIMER_TEST_CASES)
  add_test(NAME defense-timer.${test_case} COMMAND cpp-defense-timer-tests ${test_case})
endforeach()

set(BUILD_RUNNER_TEST_CASES
  success compile-failure test-failure unsupported special-path timeout)
foreach(test_case IN LISTS BUILD_RUNNER_TEST_CASES)
  add_test(NAME build-runner.${test_case} COMMAND cpp-defense-build-runner-tests ${test_case})
endforeach()

set(DEFENSE_SESSION_TEST_CASES
  start retry-success expired no-active-session stable-finished-time
  check-timeout)
foreach(test_case IN LISTS DEFENSE_SESSION_TEST_CASES)
  add_test(NAME defense-session.${test_case} COMMAND cpp-defense-session-tests ${test_case})
endforeach()

foreach(test_case IN ITEMS override platforms fallback)
  add_test(NAME runtime-path.${test_case}
           COMMAND cpp-defense-runtime-path-tests ${test_case})
endforeach()

foreach(test_case IN ITEMS command-parser eof-finalizes)
  add_test(NAME ui.${test_case} COMMAND cpp-defense-ui-tests ${test_case})
endforeach()

set(CPACK_PACKAGE_NAME "CppDefense")
set(CPACK_PACKAGE_VENDOR "CppDefense")
set(CPACK_PACKAGE_VERSION ${PROJECT_VERSION})
set(CPACK_PACKAGE_FILE_NAME
    "cpp-defense-${PROJECT_VERSION}-${CMAKE_SYSTEM_NAME}-${CMAKE_SYSTEM_PROCESSOR}")
set(CPACK_RESOURCE_FILE_LICENSE "${PROJECT_SOURCE_DIR}/LICENSE")
if (WIN32)
  set(CPACK_GENERATOR ZIP)
else()
  set(CPACK_GENERATOR TGZ)
endif()
include(CPack)

if(CPP_DEFENSE_BUILD_WORKER)
  add_executable(cpp-defense-worker-tests tests/worker_test.cpp)
  target_link_libraries(cpp-defense-worker-tests PRIVATE cpp-defense-worker-adapter)
  add_test(NAME worker.unit COMMAND cpp-defense-worker-tests)
  find_package(Python3 COMPONENTS Interpreter)
  if(Python3_Interpreter_FOUND)
    add_test(NAME worker.protocol COMMAND ${Python3_EXECUTABLE}
      ${PROJECT_SOURCE_DIR}/tests/worker_protocol_test.py
      $<TARGET_FILE:cpp-defense-worker> ${PROJECT_SOURCE_DIR})
  endif()
  find_program(CPP_DEFENSE_GO go)
  if(CPP_DEFENSE_GO)
    add_test(NAME worker.go-client COMMAND ${CPP_DEFENSE_GO} run
      ${PROJECT_SOURCE_DIR}/tests/worker_smoke.go $<TARGET_FILE:cpp-defense-worker>)
    set_tests_properties(worker.go-client PROPERTIES TIMEOUT 120)
  endif()
endif()
