cmake_minimum_required(VERSION 3.20)

project(CppDefense VERSION 0.1.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 23)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

add_executable(cpp-defense
  apps/cli/main.cpp
)

target_include_directories(cpp-defense
  PRIVATE
    ${PROJECT_SOURCE_DIR}/include
)

if (MSVC)
  target_compile_options(cpp-defense PRIVATE /W4)
else()
  target_compile_options(cpp-defense PRIVATE -Wall -Wextra -Wpedantic)
endif()