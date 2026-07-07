cmake_minimum_required(VERSION 3.20)

project(CppDefense VERSION 0.2.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 23)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

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


# =========================
# Tests
# =========================

# enable_testing()

# add_subdirectory(tests)