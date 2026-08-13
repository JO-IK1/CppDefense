#pragma once

#include <filesystem>
#include <string>
#include <system_error>
#include <utility>

namespace cpp_defense {

enum class ScanErrorType {
  kScanRootAccessFailed,
  kScanRootNotFound,
  kScanRootIsSymlink,
  kScanRootNotDirectory,
  kEntryStatusFailed,
  kTraversalFailed,
};

struct ScanError {
  ScanErrorType type;
  std::string message;
  std::filesystem::path problematic_path;
  std::error_code system_error;

  ScanError(ScanErrorType error_type, std::string error_message,
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

inline ScanError ScanRootAccessFailed(const std::filesystem::path& path,
                                      const std::error_code& error_code) {
  return ScanError(ScanErrorType::kScanRootAccessFailed,
                   "Failed to access scan root", path, error_code);
}

inline ScanError ScanRootNotFound(const std::filesystem::path& path) {
  return ScanError(ScanErrorType::kScanRootNotFound,
                   "Scan root does not exist", path);
}

inline ScanError ScanRootIsSymlink(const std::filesystem::path& path) {
  return ScanError(ScanErrorType::kScanRootIsSymlink,
                   "Scan root must not be a symbolic link", path);
}

inline ScanError ScanRootNotDirectory(const std::filesystem::path& path) {
  return ScanError(ScanErrorType::kScanRootNotDirectory,
                   "Scan root is not a directory", path);
}

inline ScanError EntryStatusFailed(const std::filesystem::path& path,
                                   const std::error_code& error_code) {
  return ScanError(ScanErrorType::kEntryStatusFailed,
                   "Failed to retrieve directory entry status", path,
                   error_code);
}

inline ScanError TraversalFailed(const std::filesystem::path& path,
                                 const std::error_code& error_code) {
  return ScanError(ScanErrorType::kTraversalFailed,
                   "Recursive directory traversal failed", path, error_code);
}

}  // namespace cpp_defense
