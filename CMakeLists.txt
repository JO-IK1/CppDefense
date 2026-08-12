cmake_minimum_required(VERSION 3.20)

project(CppDefense VERSION 0.4.0 LANGUAGES CXX)

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
  src/infrastructure/simple_function_parser.cpp
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

target_link_libraries(cpp-defense-tests
  PRIVATE
    cpp-defense-lib
)

if (MSVC)
  target_compile_options(cpp-defense-tests PRIVATE /W4)
else()
  target_compile_options(cpp-defense-tests PRIVATE -Wall -Wextra -Wpedantic)
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
