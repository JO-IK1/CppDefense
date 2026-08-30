#include "cpp_defense/infrastructure/build_runner.hpp"

#include <algorithm>
#include <chrono>
#include <fstream>
#include <iterator>
#include <string>
#include <system_error>
#include <utility>
#include <vector>

#include "process_runner.hpp"

namespace cpp_defense {
namespace {

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
    const std::string& executable,
    const std::vector<std::string>& arguments,
    const std::filesystem::path& log_path,
    std::chrono::milliseconds timeout) {
  const auto process = process_runner::Run(
      executable, arguments, log_path, timeout);
  if (!process) {
    return std::unexpected(process.error());
  }

  std::string process_output;
  std::error_code log_error;
  const bool log_exists = std::filesystem::exists(log_path, log_error);
  if (log_error) {
    return std::unexpected(BuildRunnerError(
        BuildRunnerErrorType::kFailedToReadLog,
        "Failed to inspect build log", log_path));
  }
  if (log_exists) {
    const auto output = ReadLog(log_path);
    if (!output) {
      return std::unexpected(output.error());
    }
    process_output = *output;
  }

  BuildStepResult result{
      .attempted = true,
      .succeeded = process->exit_code == 0 && !process->timed_out,
      .timed_out = process->timed_out,
      .exit_code = process->exit_code,
      .output = std::move(process_output),
  };
  if (result.timed_out) {
    if (!result.output.empty() && result.output.back() != '\n') {
      result.output.push_back('\n');
    }
    result.output += "CppDefense: command timed out.\n";
  }
  return result;
}

std::chrono::milliseconds Remaining(
    std::chrono::steady_clock::time_point deadline) {
  return std::max(
      std::chrono::duration_cast<std::chrono::milliseconds>(
          deadline - std::chrono::steady_clock::now()),
      std::chrono::milliseconds::zero());
}

}  // namespace

std::expected<BuildResult, BuildRunnerError> BuildRunner::Run(
    const std::filesystem::path& project_path,
    const std::filesystem::path& build_path,
    const std::filesystem::path& logs_path,
    std::chrono::milliseconds timeout) const {
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
  const auto deadline = std::chrono::steady_clock::now() + timeout;

  const auto configure = RunStep(
      "cmake",
      {"-S", project_path.string(), "-B", build_path.string(),
       "-DCMAKE_BUILD_TYPE=Release"},
      logs_path / "configure.log", Remaining(deadline));
  if (!configure) {
    return std::unexpected(configure.error());
  }
  result.configure = *configure;
  if (!result.configure.succeeded) {
    return result;
  }

  const auto build = RunStep(
      "cmake", {"--build", build_path.string(), "--config", "Release"},
      logs_path / "build.log", Remaining(deadline));
  if (!build) {
    return std::unexpected(build.error());
  }
  result.build = *build;
  if (!result.build.succeeded) {
    return result;
  }

  const auto tests = RunStep(
      "ctest",
      {"--test-dir", build_path.string(), "--build-config", "Release",
       "--output-on-failure"},
      logs_path / "tests.log", Remaining(deadline));
  if (!tests) {
    return std::unexpected(tests.error());
  }
  result.tests = *tests;

  return result;
}

}  // namespace cpp_defense
