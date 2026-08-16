#pragma once

#include <filesystem>
#include <string>
#include <system_error>
#include <utility>

namespace cpp_defense {

enum class ResultFileErrorType {
  kSourceOpenFailed,
  kSourceReadFailed,
  kInvalidEntityRange,
  kBodyBoundaryMismatch,
  kOpenFailed,
  kWriteFailed,
  kReadFailed,
};

struct ResultFileError {
  ResultFileErrorType type;
  std::string message;
  std::filesystem::path problematic_path;
  std::error_code system_error;

  ResultFileError(ResultFileErrorType error_type, std::string error_message,
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

inline ResultFileError ResultSourceOpenFailed(const std::filesystem::path& path,
                                              const std::error_code& error_code = {}) {
  return ResultFileError(ResultFileErrorType::kSourceOpenFailed,
                         "Failed to open selected source file", path, error_code);
}

inline ResultFileError ResultSourceReadFailed(const std::filesystem::path& path,
                                              const std::error_code& error_code = {}) {
  return ResultFileError(ResultFileErrorType::kSourceReadFailed,
                         "Failed to read selected source file", path, error_code);
}

inline ResultFileError ResultInvalidEntityRange(const std::filesystem::path& path) {
  return ResultFileError(ResultFileErrorType::kInvalidEntityRange,
                         "Selected entity has an invalid source range", path);
}

inline ResultFileError ResultBodyBoundaryMismatch(const std::filesystem::path& path) {
  return ResultFileError(ResultFileErrorType::kBodyBoundaryMismatch,
                         "Selected entity body does not match source braces", path);
}

inline ResultFileError ResultFileOpenFailed(const std::filesystem::path& path,
                                            const std::error_code& error_code = {}) {
  return ResultFileError(ResultFileErrorType::kOpenFailed,
                         "Failed to open result file", path, error_code);
}

inline ResultFileError ResultFileWriteFailed(const std::filesystem::path& path,
                                             const std::error_code& error_code = {}) {
  return ResultFileError(ResultFileErrorType::kWriteFailed,
                         "Failed to write result file", path, error_code);
}

inline ResultFileError ResultFileReadFailed(const std::filesystem::path& path,
                                            const std::error_code& error_code = {}) {
  return ResultFileError(ResultFileErrorType::kReadFailed,
                         "Failed to read result file", path, error_code);
}

}  // namespace cpp_defense
