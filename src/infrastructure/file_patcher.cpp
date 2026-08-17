#include "cpp_defense/infrastructure/file_patcher.hpp"

#include <fstream>
#include <iterator>
#include <string>

namespace cpp_defense {
namespace {

bool StartsWithParentTraversal(const std::filesystem::path& path) {
  return !path.empty() && *path.begin() == "..";
}

}  // namespace

std::expected<std::filesystem::path, FilePatcherError> FilePatcher::Patch(
    const CodeEntityInfo& entity,
    const std::filesystem::path& cached_project_path,
    const std::filesystem::path& check_project_path,
    std::string_view replacement) const {
  const std::filesystem::path relative_path =
      entity.file_path.lexically_relative(cached_project_path);

  if (relative_path.empty() || StartsWithParentTraversal(relative_path)) {
    return std::unexpected(FilePatcherError(
        FilePatcherErrorType::kEntityOutsideCachedProject,
        "Selected entity is outside the cached project",
        entity.file_path));
  }

  const std::filesystem::path target_path =
      (check_project_path / relative_path).lexically_normal();

  std::error_code error_code;
  if (!std::filesystem::is_regular_file(target_path, error_code) || error_code) {
    return std::unexpected(FilePatcherError(
        FilePatcherErrorType::kTargetFileMissing,
        "Target source file does not exist in check workspace",
        target_path));
  }

  std::ifstream input(target_path, std::ios::binary);
  if (!input.is_open()) {
    return std::unexpected(FilePatcherError(
        FilePatcherErrorType::kOpenFailed,
        "Failed to open check source file",
        target_path));
  }

  std::string source{
      std::istreambuf_iterator<char>(input),
      std::istreambuf_iterator<char>()};

  if (input.bad()) {
    return std::unexpected(FilePatcherError(
        FilePatcherErrorType::kReadFailed,
        "Failed to read check source file",
        target_path));
  }

  if (entity.start_offset > entity.end_offset ||
      entity.end_offset > source.size()) {
    return std::unexpected(FilePatcherError(
        FilePatcherErrorType::kInvalidEntityRange,
        "Selected entity has an invalid source range",
        target_path));
  }

  source.replace(entity.start_offset,
                 entity.end_offset - entity.start_offset,
                 replacement);

  input.close();
  std::ofstream output(target_path, std::ios::binary | std::ios::trunc);
  if (!output.is_open()) {
    return std::unexpected(FilePatcherError(
        FilePatcherErrorType::kWriteFailed,
        "Failed to open check source file for writing",
        target_path));
  }

  output.write(source.data(), static_cast<std::streamsize>(source.size()));
  if (!output) {
    return std::unexpected(FilePatcherError(
        FilePatcherErrorType::kWriteFailed,
        "Failed to write patched source file",
        target_path));
  }

  return target_path;
}

}  // namespace cpp_defense
