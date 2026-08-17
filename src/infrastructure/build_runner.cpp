#include "cpp_defense/infrastructure/build_runner.hpp"

#include <cstdlib>
#include <fstream>
#include <iterator>
#include <string>
#include <system_error>

namespace cpp_defense {
namespace {

std::string Quote(const std::filesystem::path& path) {
  return '"' + path.string() + '"';
}

std::expected<std::string, BuildRunnerError> ReadLog(
    const std::filesystem::path& log_path) {
  std::ifstream input(log_path, std::ios::binary);
  if (!input.is_open()) {
    return std::unexpected(BuildRunnerError(
        BuildRunnerErrorType::kFailedToReadLog,
        "Failed to open build log",
        log_path));
  }

  std::string contents{
      std::istreambuf_iterator<char>(input),
      std::istreambuf_iterator<char>()};

  if (input.bad()) {
    return std::unexpected(BuildRunnerError(
        BuildRunnerErrorType::kFailedToReadLog,
        "Failed to read build log",
        log_path));
  }

  return contents;
}

std::expected<BuildStepResult, BuildRunnerError> RunStep(
    const std::string& command,
    const std::filesystem::path& log_path) {
  const std::string redirected =
      command + " > " + Quote(log_path) + " 2>&1";

  const int exit_code = std::system(redirected.c_str());
  if (exit_code == -1) {
    return std::unexpected(BuildRunnerError(
        BuildRunnerErrorType::kFailedToRunCommand,
        "Failed to start build command"));
  }

  const auto output = ReadLog(log_path);
  if (!output) {
    return std::unexpected(output.error());
  }

  return BuildStepResult{
      .attempted = true,
      .succeeded = exit_code == 0,
      .exit_code = exit_code,
      .output = *output,
  };
}

}  // namespace

std::expected<BuildResult, BuildRunnerError> BuildRunner::Run(
    const std::filesystem::path& project_path,
    const std::filesystem::path& build_path,
    const std::filesystem::path& logs_path) const {
  std::error_code error_code;
  if (!std::filesystem::is_directory(project_path, error_code) || error_code) {
    return std::unexpected(BuildRunnerError(
        BuildRunnerErrorType::kProjectMissing,
        "Check project directory does not exist",
        project_path));
  }

  const std::filesystem::path cmake_file = project_path / "CMakeLists.txt";
  if (!std::filesystem::is_regular_file(cmake_file, error_code) || error_code) {
    return std::unexpected(BuildRunnerError(
        BuildRunnerErrorType::kUnsupportedBuildSystem,
        "CppDefense currently supports CMake projects only",
        project_path));
  }

  std::filesystem::create_directories(build_path, error_code);
  if (error_code) {
    return std::unexpected(BuildRunnerError(
        BuildRunnerErrorType::kFailedToCreateBuildDirectory,
        "Failed to create check build directory",
        build_path));
  }

  std::filesystem::create_directories(logs_path, error_code);
  if (error_code) {
    return std::unexpected(BuildRunnerError(
        BuildRunnerErrorType::kFailedToCreateLogDirectory,
        "Failed to create build log directory",
        logs_path));
  }

  BuildResult result;

  const auto configure = RunStep(
      "cmake -S " + Quote(project_path) +
          " -B " + Quote(build_path) +
          " -DCMAKE_BUILD_TYPE=Release",
      logs_path / "configure.log");
  if (!configure) {
    return std::unexpected(configure.error());
  }
  result.configure = *configure;
  if (!result.configure.succeeded) {
    return result;
  }

  const auto build = RunStep(
      "cmake --build " + Quote(build_path) + " --config Release",
      logs_path / "build.log");
  if (!build) {
    return std::unexpected(build.error());
  }
  result.build = *build;
  if (!result.build.succeeded) {
    return result;
  }

  const auto tests = RunStep(
      "ctest --test-dir " + Quote(build_path) +
          " --build-config Release --output-on-failure",
      logs_path / "tests.log");
  if (!tests) {
    return std::unexpected(tests.error());
  }
  result.tests = *tests;

  return result;
}

}  // namespace cpp_defense
