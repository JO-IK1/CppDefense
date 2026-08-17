cmake_minimum_required(VERSION 3.20)
project(LabworkSimple LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

enable_testing()

add_library(math_utils src/math_utils.cpp)
target_include_directories(math_utils PUBLIC include)

add_executable(labwork_simple src/main.cpp)
target_link_libraries(labwork_simple PRIVATE math_utils)

add_executable(math_utils_test tests/math_utils_test.cpp)
target_link_libraries(math_utils_test PRIVATE math_utils)
add_test(NAME math-utils COMMAND math_utils_test)
