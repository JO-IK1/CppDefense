#pragma once

#include <filesystem>
#include <string>
#include <system_error>
#include <utility>

namespace cpp_defense {

enum class SourceFileErrorType {
  kOpenFailed,
  kReadFailed,
};

struct SourceFileError {
  SourceFileErrorType type;
  std::string message;
  std::filesystem::path problematic_path;
  std::error_code system_error;

  SourceFileError(SourceFileErrorType error_type, std::string error_message,
                  std::filesystem::path path, std::error_code error_code = {})
      : type(error_type), message(std::move(error_message)),
        problematic_path(std::move(path)), system_error(error_code) {}

  std::string FullMessage() const {
    std::string full = message;

    if (!problematic_path.empty()) {
      full += ". Path: " + problematic_path.string();
    }

    if (system_error) {
      full += ". System error: " + system_error.message() +
              " (code: " + std::to_string(system_error.value()) + ")";
    }

    return full;
  }
};

inline SourceFileError SourceFileOpenFailed(const std::filesystem::path& path,
                                            const std::error_code& error_code = {}) {
  return SourceFileError(SourceFileErrorType::kOpenFailed,
                         "Failed to open source file", path, error_code);
}

inline SourceFileError SourceFileReadFailed(const std::filesystem::path& path,
                                            const std::error_code& error_code = {}) {
  return SourceFileError(SourceFileErrorType::kReadFailed,
                         "Failed to read source file", path, error_code);
}

}  // namespace cpp_defense
