#include <array>
#include <chrono>
#include <filesystem>
#include <fstream>
#include <iostream>
#include <string_view>

#include "cpp_defense/infrastructure/build_runner.hpp"

namespace {
namespace fs = std::filesystem;
using cpp_defense::BuildRunner;
using cpp_defense::BuildRunnerErrorType;
constexpr int kSuccess = 0;
constexpr int kFailure = 1;

class TempDir {
 public:
  TempDir() {
    path_ = fs::temp_directory_path() /
            ("cpp-defense-build-runner-" + std::to_string(
                std::chrono::steady_clock::now().time_since_epoch().count()));
    fs::create_directories(path_);
  }
  ~TempDir() { std::error_code ec; fs::remove_all(path_, ec); }
  const fs::path& path() const { return path_; }
 private:
  fs::path path_;
};

void Write(const fs::path& path, std::string_view text) {
  fs::create_directories(path.parent_path());
  std::ofstream out(path);
  out << text;
}

bool Expect(bool condition, std::string_view message) {
  if (!condition) std::cerr << "FAILED: " << message << '\n';
  return condition;
}

void WriteProject(const fs::path& project, std::string_view source) {
  Write(project / "CMakeLists.txt",
        "cmake_minimum_required(VERSION 3.20)\n"
        "project(Sample LANGUAGES CXX)\n"
        "enable_testing()\n"
        "add_executable(sample_test main.cpp)\n"
        "add_test(NAME sample COMMAND sample_test)\n");
  Write(project / "main.cpp", source);
}

bool TestSuccess() {
  TempDir temp;
  const fs::path project = temp.path() / "project";
  WriteProject(project, "int main() { return 0; }\n");
  BuildRunner runner;
  const auto result = runner.Run(project, temp.path() / "build", temp.path() / "logs");
  return Expect(result.has_value(), "runner returns a result") &&
         Expect(result->success(), "configure, build and tests succeed");
}

bool TestCompileFailure() {
  TempDir temp;
  const fs::path project = temp.path() / "project";
  WriteProject(project, "int main( { return 0; }\n");
  BuildRunner runner;
  const auto result = runner.Run(project, temp.path() / "build", temp.path() / "logs");
  return Expect(result.has_value(), "compile failure is a normal build result") &&
         Expect(result->configure.succeeded, "configure succeeds") &&
         Expect(!result->build.succeeded, "build failure is reported") &&
         Expect(!result->tests.attempted, "tests are skipped after build failure");
}

bool TestTestFailure() {
  TempDir temp;
  const fs::path project = temp.path() / "project";
  WriteProject(project, "int main() { return 1; }\n");
  BuildRunner runner;
  const auto result = runner.Run(project, temp.path() / "build", temp.path() / "logs");
  return Expect(result.has_value(), "test failure is a normal build result") &&
         Expect(result->build.succeeded, "project builds") &&
         Expect(result->tests.attempted && !result->tests.succeeded,
                "test failure is reported");
}

bool TestUnsupported() {
  TempDir temp;
  const fs::path project = temp.path() / "project";
  fs::create_directories(project);
  BuildRunner runner;
  const auto result = runner.Run(project, temp.path() / "build", temp.path() / "logs");
  return Expect(!result.has_value(), "project without CMake is rejected") &&
         Expect(result.error().type == BuildRunnerErrorType::kUnsupportedBuildSystem,
                "unsupported build system error is reported");
}

struct TestCase { std::string_view name; bool (*fn)(); };
constexpr std::array<TestCase, 4> kTests{{
    {"success", TestSuccess},
    {"compile-failure", TestCompileFailure},
    {"test-failure", TestTestFailure},
    {"unsupported", TestUnsupported},
}};
}

int main(int argc, char* argv[]) {
  if (argc != 2) return kFailure;
  for (const auto& test : kTests) {
    if (test.name == argv[1]) return test.fn() ? kSuccess : kFailure;
  }
  std::cerr << "Unknown test case: " << argv[1] << '\n';
  return kFailure;
}
