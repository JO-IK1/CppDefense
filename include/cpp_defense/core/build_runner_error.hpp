#pragma once

#include <filesystem>
#include <string>
#include <utility>

namespace cpp_defense {

enum class BuildRunnerErrorType {
  kProjectMissing,
  kUnsupportedBuildSystem,
  kFailedToCreateBuildDirectory,
  kFailedToCreateLogDirectory,
  kFailedToRunCommand,
  kFailedToReadLog,
};

struct BuildRunnerError {
  BuildRunnerErrorType type;
  std::string message;
  std::filesystem::path problematic_path;

  BuildRunnerError(BuildRunnerErrorType error_type, std::string error_message,
                   std::filesystem::path path = {})
      : type(error_type), message(std::move(error_message)),
        problematic_path(std::move(path)) {}

  std::string FullMessage() const {
    if (problematic_path.empty()) {
      return message;
    }
    return message + ". Path: " + problematic_path.string();
  }
};

}  // namespace cpp_defense
