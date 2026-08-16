#pragma once

#include <filesystem>
#include <string>
#include <system_error>
#include <utility>

namespace cpp_defense {

enum class FileMaskerErrorType {
  kOpenFailed,
  kReadFailed,
  kInvalidEntityRange,
  kBodyBoundaryMismatch,
  kWriteFailed,
};

struct FileMaskerError {
  FileMaskerErrorType type;
  std::string message;
  std::filesystem::path problematic_path;
  std::error_code system_error;

  FileMaskerError(FileMaskerErrorType error_type, std::string error_message,
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

inline FileMaskerError FileMaskerOpenFailed(const std::filesystem::path& path,
                                            const std::error_code& error_code = {}) {
  return FileMaskerError(FileMaskerErrorType::kOpenFailed,
                         "Failed to open source file for masking", path, error_code);
}

inline FileMaskerError FileMaskerReadFailed(const std::filesystem::path& path,
                                            const std::error_code& error_code = {}) {
  return FileMaskerError(FileMaskerErrorType::kReadFailed,
                         "Failed to read source file for masking", path, error_code);
}

inline FileMaskerError FileMaskerInvalidEntityRange(const std::filesystem::path& path) {
  return FileMaskerError(FileMaskerErrorType::kInvalidEntityRange,
                         "Selected entity has an invalid body range", path);
}

inline FileMaskerError FileMaskerBodyBoundaryMismatch(const std::filesystem::path& path) {
  return FileMaskerError(FileMaskerErrorType::kBodyBoundaryMismatch,
                         "Selected entity body does not match source braces", path);
}

inline FileMaskerError FileMaskerWriteFailed(const std::filesystem::path& path,
                                             const std::error_code& error_code = {}) {
  return FileMaskerError(FileMaskerErrorType::kWriteFailed,
                         "Failed to write masked source file", path, error_code);
}

}  // namespace cpp_defense
