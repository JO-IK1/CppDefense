#pragma once

#include <filesystem>
#include <string>
#include <utility>

namespace cpp_defense {

enum class FilePatcherErrorType {
  kEntityOutsideCachedProject,
  kTargetFileMissing,
  kOpenFailed,
  kReadFailed,
  kInvalidEntityRange,
  kWriteFailed,
};

struct FilePatcherError {
  FilePatcherErrorType type;
  std::string message;
  std::filesystem::path problematic_path;

  FilePatcherError(FilePatcherErrorType error_type, std::string error_message,
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
