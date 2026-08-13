#include "cpp_defense/infrastructure/project_scanner.hpp"

#include <algorithm>
#include <array>
#include <cctype>
#include <expected>
#include <filesystem>
#include <string>
#include <string_view>
#include <system_error>
#include <vector>

namespace cpp_defense {
namespace {

constexpr std::array<std::string_view, 8> kSupportedExtensions{
    ".c", ".cc", ".cpp", ".cxx", ".h", ".hh", ".hpp", ".hxx"};

constexpr std::array<std::string_view, 7> kExcludedDirectoryNames{
    ".git",  ".idea",             ".vscode", "build",
    "cache", "cmake-build-debug", "cmake-build-release"};

bool IsSupportedSourceFile(const std::filesystem::path& file_path) {
  std::string extension = file_path.extension().string();
  std::transform(extension.begin(), extension.end(), extension.begin(),
                 [](char character) {
                   const unsigned char unsigned_character =
                       static_cast<unsigned char>(character);
                   return static_cast<char>(std::tolower(unsigned_character));
                 });

  return std::find(kSupportedExtensions.begin(), kSupportedExtensions.end(),
                   extension) != kSupportedExtensions.end();
}

bool ShouldSkipDirectory(const std::filesystem::path& directory_path) {
  const std::string directory_name = directory_path.filename().string();
  return std::find(kExcludedDirectoryNames.begin(),
                   kExcludedDirectoryNames.end(),
                   directory_name) != kExcludedDirectoryNames.end();
}

}  // namespace

std::expected<SourceFilePaths, ScanError> ProjectScanner::FindSourceFiles(
    const std::filesystem::path& cached_project_path) const {
  SourceFilePaths source_file_paths;
  std::error_code error_code;

  const std::filesystem::file_status root_status =
      std::filesystem::symlink_status(cached_project_path, error_code);

  if (error_code == std::errc::no_such_file_or_directory) {
    return std::unexpected(ScanRootNotFound(cached_project_path));
  }

  if (error_code) {
    return std::unexpected(ScanRootAccessFailed(cached_project_path, error_code));
  }

  if (!std::filesystem::exists(root_status)) {
    return std::unexpected(ScanRootNotFound(cached_project_path));
  }

  if (std::filesystem::is_symlink(root_status)) {
    return std::unexpected(ScanRootIsSymlink(cached_project_path));
  }

  if (!std::filesystem::is_directory(root_status)) {
    return std::unexpected(ScanRootNotDirectory(cached_project_path));
  }

  std::filesystem::recursive_directory_iterator iterator(
      cached_project_path, std::filesystem::directory_options::none,
      error_code);
  if (error_code) {
    return std::unexpected(ScanRootAccessFailed(cached_project_path, error_code));
  }
  const auto end_iterator = std::filesystem::recursive_directory_iterator{};

  while (iterator != end_iterator) {
    const auto& entry = *iterator;
    const std::filesystem::path entry_path = entry.path();

    const std::filesystem::file_status entry_status =
        entry.symlink_status(error_code);
    if (error_code) {
      return std::unexpected(EntryStatusFailed(entry_path, error_code));
    }

    if (std::filesystem::is_symlink(entry_status)) {
      // Symbolic links are skipped.
    } else if (std::filesystem::is_directory(entry_status)) {
      if (ShouldSkipDirectory(entry_path)) {
        iterator.disable_recursion_pending();
      }
    } else if (std::filesystem::is_regular_file(entry_status)) {
      if (IsSupportedSourceFile(entry_path)) {
        source_file_paths.push_back(entry_path);
      }
    }

    iterator.increment(error_code);
    if (error_code) {
      return std::unexpected(TraversalFailed(entry_path, error_code));
    }
  }

  std::sort(source_file_paths.begin(), source_file_paths.end());

  return source_file_paths;
}

}  // namespace cpp_defense
