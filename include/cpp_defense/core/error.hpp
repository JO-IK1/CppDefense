#pragma once

#include <filesystem>
#include <string>
#include <system_error>
#include <utility>

namespace cpp_defense {

enum class CacheErrorType {
  kSourcePathMissing,
  kCppDefenseRootMissing,
  kPathIsNotDirectory,
  kCannotDetermineAbsolutePath,
  kCannotDetermineProjectName,
  kSourceProjectEqualsCppDefense,
  kSourceProjectInsideCache,
  kCacheInsideSourceProject,
  kDangerousCleanupPath,
  kFailedToRemoveOldCache,
  kFailedToCreateDirectory,
  kFailedToReadSourceFile,
  kFailedToCopyFile,
  kSymlinkDetected,
  kOtherFilesystemError,
};

struct CacheError {
  CacheErrorType type;
  std::string message;
  std::filesystem::path problematic_path;
  std::error_code system_error;

  CacheError(CacheErrorType error_type, std::string error_message,
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

inline CacheError SourcePathMissing(const std::filesystem::path& path) {
  return CacheError(CacheErrorType::kSourcePathMissing,
                    "Source path does not exist", path);
}

inline CacheError CppDefenseRootMissing(const std::filesystem::path& path) {
  return CacheError(CacheErrorType::kCppDefenseRootMissing,
                    "CppDefense root path does not exist", path);
}

inline CacheError PathIsNotDirectory(const std::filesystem::path& path) {
  return CacheError(CacheErrorType::kPathIsNotDirectory,
                    "Path is not a directory", path);
}

inline CacheError CannotDetermineAbsolutePath(
    const std::filesystem::path& path, const std::error_code& error_code = {}) {
  return CacheError(CacheErrorType::kCannotDetermineAbsolutePath,
                    "Cannot determine absolute path", path, error_code);
}

inline CacheError CannotDetermineProjectName(
    const std::filesystem::path& path) {
  return CacheError(CacheErrorType::kCannotDetermineProjectName,
                    "Cannot determine source project directory name", path);
}

inline CacheError SourceProjectEqualsCppDefense(
    const std::filesystem::path& path) {
  return CacheError(CacheErrorType::kSourceProjectEqualsCppDefense,
                    "Source project is CppDefense itself", path);
}

inline CacheError SourceProjectInsideCache(
    const std::filesystem::path& path) {
  return CacheError(CacheErrorType::kSourceProjectInsideCache,
                    "Source project is located inside the cache directory", path);
}

inline CacheError CacheInsideSourceProject(
    const std::filesystem::path& path) {
  return CacheError(CacheErrorType::kCacheInsideSourceProject,
                    "Cache directory is located inside the source project", path);
}

inline CacheError DangerousCleanupPath(const std::filesystem::path& path) {
  return CacheError(CacheErrorType::kDangerousCleanupPath,
                    "Cleanup path is outside the allowed cache location", path);
}

inline CacheError FailedToRemoveOldCache(
    const std::filesystem::path& path, const std::error_code& error_code) {
  return CacheError(CacheErrorType::kFailedToRemoveOldCache,
                    "Failed to remove old cache directory", path, error_code);
}

inline CacheError FailedToCreateDirectory(
    const std::filesystem::path& path, const std::error_code& error_code) {
  return CacheError(CacheErrorType::kFailedToCreateDirectory,
                    "Failed to create directory", path, error_code);
}

inline CacheError FailedToReadSourceFile(
    const std::filesystem::path& path, const std::error_code& error_code) {
  return CacheError(CacheErrorType::kFailedToReadSourceFile,
                    "Failed to read source file", path, error_code);
}

inline CacheError FailedToCopyFile(const std::filesystem::path& path,
                                   const std::error_code& error_code) {
  return CacheError(CacheErrorType::kFailedToCopyFile,
                    "Failed to copy file", path, error_code);
}

inline CacheError SymlinkDetected(const std::filesystem::path& path) {
  return CacheError(
      CacheErrorType::kSymlinkDetected,
      "Symbolic link detected and is not allowed in cache operation", path);
}

inline CacheError OtherFilesystemError(
    const std::filesystem::path& path, const std::error_code& error_code) {
  return CacheError(CacheErrorType::kOtherFilesystemError,
                    "Other filesystem error occurred", path, error_code);
}

}  // namespace cpp_defense
