#pragma once

#include <chrono>
#include <expected>
#include <filesystem>

#include "cpp_defense/core/build_result.hpp"
#include "cpp_defense/core/build_runner_error.hpp"

namespace cpp_defense {

class BuildRunner {
 public:
  std::expected<BuildResult, BuildRunnerError> Run(
      const std::filesystem::path& project_path,
      const std::filesystem::path& build_path,
      const std::filesystem::path& logs_path,
      std::chrono::milliseconds timeout = std::chrono::minutes(30)) const;
};

}  // namespace cpp_defense
