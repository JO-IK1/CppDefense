#include "cpp_defense/infrastructure/workspace_cache.hpp"

#include <expected>
#include <filesystem>
#include <string>
#include <string_view>
#include <system_error>
#include <utility>

namespace cpp_defense {
namespace {

std::expected<std::filesystem::path, CacheError> NormalizeDirectoryPath(
    const std::filesystem::path& path, std::string_view path_description,
    CacheErrorType missing_path_error) {
  if (path.empty()) {
    return std::unexpected(CacheError(
        missing_path_error,
        std::string(path_description) + " path is empty", path));
  }

  std::error_code error_code;
  const std::filesystem::path absolute_path =
      std::filesystem::absolute(path, error_code);
  if (error_code) {
    return std::unexpected(
        CannotDetermineAbsolutePath(path, error_code));
  }

  const bool exists = std::filesystem::exists(absolute_path, error_code);
  if (error_code) {
    return std::unexpected(
        OtherFilesystemError(absolute_path, error_code));
  }

  if (!exists) {
    return std::unexpected(CacheError(
        missing_path_error,
        std::string(path_description) + " path does not exist",
        absolute_path));
  }

  const bool is_directory = std::filesystem::is_directory(absolute_path, error_code);
  if (error_code) {
    return std::unexpected(
        OtherFilesystemError(absolute_path, error_code));
  }

  if (!is_directory) {
    return std::unexpected(PathIsNotDirectory(absolute_path));
  }

  const std::filesystem::path canonical_path = std::filesystem::canonical(absolute_path, error_code);
  if (error_code) {
    return std::unexpected(
        CannotDetermineAbsolutePath(absolute_path, error_code));
  }

  return canonical_path;
}

}  // namespace

WorkspaceCache::WorkspaceCache(std::filesystem::path cpp_defense_root_path)
    : cpp_defense_root_path_(std::move(cpp_defense_root_path)) {}

std::expected<Workspace, CacheError> WorkspaceCache::CalculateWorkspace(
    const std::filesystem::path& source_project_path) const {
  const auto normalized_cpp_defense_root = NormalizeDirectoryPath(
      cpp_defense_root_path_, "CppDefense root",
      CacheErrorType::kCppDefenseRootMissing);
  if (!normalized_cpp_defense_root) {
    return std::unexpected(normalized_cpp_defense_root.error());
  }

  const auto normalized_source_project = NormalizeDirectoryPath(
      source_project_path, "Source project",
      CacheErrorType::kSourcePathMissing);
  if (!normalized_source_project) {
    return std::unexpected(normalized_source_project.error());
  }

  const std::filesystem::path project_name = normalized_source_project->filename();
  if (project_name.empty() || project_name == "." || project_name == "..") {
    return std::unexpected(
        CannotDetermineProjectName(*normalized_source_project));
  }

  Workspace workspace;
  workspace.cpp_defense_root_path = *normalized_cpp_defense_root;
  workspace.source_project_path = *normalized_source_project;
  workspace.cache_root_path = (workspace.cpp_defense_root_path / "cache").lexically_normal();
  workspace.session_root_path = (workspace.cache_root_path / "current").lexically_normal();
  workspace.project_container_path = (workspace.session_root_path / "project").lexically_normal();
  workspace.cached_project_path = (workspace.project_container_path / project_name).lexically_normal();
  workspace.build_path = (workspace.session_root_path / "build").lexically_normal();
  workspace.metadata_path = (workspace.session_root_path / "metadata").lexically_normal();
  workspace.logs_path = (workspace.session_root_path / "logs").lexically_normal();
  workspace.result_path = (workspace.session_root_path / "result.txt").lexically_normal();
  workspace.session_data_path = (workspace.metadata_path / "session.txt").lexically_normal();
  workspace.original_fragment_path = (workspace.metadata_path / "original_fragment.txt").lexically_normal();

  return workspace;
}

}  // namespace cpp_defense
