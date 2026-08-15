#include "cpp_defense/infrastructure/project_scanner.hpp"

#include <algorithm>
#include <array>
#include <cctype>
#include <expected>
#include <filesystem>
#include <string>
#include <string_view>
#include <system_error>
#include <utility>
#include <vector>

namespace cpp_defense {
namespace {

constexpr std::array<std::string_view, 8> kSupportedExtensions{
    ".c", ".cc", ".cpp", ".cxx", ".h", ".hh", ".hpp", ".hxx"};

std::string ToLower(std::string value) {
  std::transform(value.begin(), value.end(), value.begin(),
                 [](char character) {
                   const unsigned char unsigned_character =
                       static_cast<unsigned char>(character);
                   return static_cast<char>(std::tolower(unsigned_character));
                 });

  return value;
}

bool IsSupportedSourceFile(const std::filesystem::path& file_path) {
  const std::string extension = ToLower(file_path.extension().string());

  return std::find(kSupportedExtensions.begin(), kSupportedExtensions.end(),
                   extension) != kSupportedExtensions.end();
}

bool ShouldSkipDirectory(
    const std::filesystem::path& directory_path,
    const ProjectScannerOptions& options) {
  const std::string directory_name = ToLower(directory_path.filename().string());

  return std::find(options.excluded_directory_names.begin(),
                   options.excluded_directory_names.end(),
                   directory_name) !=
         options.excluded_directory_names.end();
}

}  // namespace

ProjectScanner::ProjectScanner(ProjectScannerOptions options)
    : options_(std::move(options)) {
  for (std::string& directory_name : options_.excluded_directory_names) {
    directory_name = ToLower(std::move(directory_name));
  }
}

std::expected<SourceFilePaths, ScanError> ProjectScanner::FindSourceFiles(
    const std::filesystem::path& cached_project_path) const {
  SourceFilePaths source_file_paths;
  std::error_code error_code;

  const std::filesystem::path scan_root = std::filesystem::absolute(
      cached_project_path, error_code).lexically_normal();

  if (error_code) {
    return std::unexpected(ScanRootAccessFailed(cached_project_path, error_code));
  }

  const std::filesystem::file_status root_status =
      std::filesystem::symlink_status(scan_root, error_code);

  if (error_code == std::errc::no_such_file_or_directory) {
    return std::unexpected(ScanRootNotFound(scan_root));
  }

  if (error_code) {
    return std::unexpected(ScanRootAccessFailed(scan_root, error_code));
  }

  if (!std::filesystem::exists(root_status)) {
    return std::unexpected(ScanRootNotFound(scan_root));
  }

  if (std::filesystem::is_symlink(root_status)) {
    return std::unexpected(ScanRootIsSymlink(scan_root));
  }

  if (!std::filesystem::is_directory(root_status)) {
    return std::unexpected(ScanRootNotDirectory(scan_root));
  }

  std::filesystem::recursive_directory_iterator iterator(
      scan_root, std::filesystem::directory_options::none, error_code);

  if (error_code) {
    return std::unexpected(ScanRootAccessFailed(scan_root, error_code));
  }

  const auto end_iterator = std::filesystem::recursive_directory_iterator{};

  while (iterator != end_iterator) {
    const auto& entry = *iterator;
    const std::filesystem::path entry_path =
        entry.path().lexically_normal();

    const std::filesystem::file_status entry_status =
        entry.symlink_status(error_code);

    if (error_code) {
      return std::unexpected(EntryStatusFailed(entry_path, error_code));
    }

    if (std::filesystem::is_symlink(entry_status)) {
      // Symbolic links are skipped.
    } else if (std::filesystem::is_directory(entry_status)) {
      if (ShouldSkipDirectory(entry_path, options_)) {
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
