#pragma once

#include <filesystem>
#include <string>
#include <system_error>
#include <utility>

namespace cpp_defense {

enum class CheckWorkspaceErrorType {
  kFailedToRemoveOldWorkspace,
  kFailedToCreateDirectory,
  kFailedToCopyProject,
};

struct CheckWorkspaceError {
  CheckWorkspaceErrorType type;
  std::string message;
  std::filesystem::path problematic_path;
  std::error_code system_error;

  CheckWorkspaceError(CheckWorkspaceErrorType error_type, std::string error_message,
                      std::filesystem::path path, std::error_code error_code = {})
      : type(error_type), message(std::move(error_message)),
        problematic_path(std::move(path)), system_error(error_code) {}

  std::string FullMessage() const {
    std::string full = message;
    if (!problematic_path.empty()) {
      full += ". Path: " + problematic_path.string();
    }
    if (system_error) {
      full += ". System error: " + system_error.message();
    }
    return full;
  }
};

}  // namespace cpp_defense
