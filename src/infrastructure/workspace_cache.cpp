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
  const std::filesystem::path absolute_path = std::filesystem::absolute(path, error_code);
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

bool IsSameOrDescendant(const std::filesystem::path& candidate,
                        const std::filesystem::path& ancestor) {
  auto candidate_part = candidate.begin();
  for (auto ancestor_part = ancestor.begin(); ancestor_part != ancestor.end();
       ++ancestor_part, ++candidate_part) {
    if (candidate_part == candidate.end() || *candidate_part != *ancestor_part) {
      return false;
    }
  }

  return true;
}

std::expected<void, CacheError> ValidateWorkspaceRelationships(const Workspace& workspace) {
  if (workspace.source_project_path == workspace.cpp_defense_root_path) {
    return std::unexpected(
        SourceProjectEqualsCppDefense(workspace.source_project_path));
  }

  if (IsSameOrDescendant(workspace.source_project_path,
                         workspace.cache_root_path)) {
    return std::unexpected(
        SourceProjectInsideCache(workspace.source_project_path));
  }

  if (IsSameOrDescendant(workspace.cache_root_path,
                         workspace.source_project_path)) {
    return std::unexpected(
        CacheInsideSourceProject(workspace.cache_root_path));
  }

  return {};
}

std::expected<void, CacheError> ValidateCleanupTarget(const Workspace& workspace) {
  if (workspace.cache_root_path.parent_path() !=
          workspace.cpp_defense_root_path ||
      workspace.cache_root_path.filename() != "cache" ||
      workspace.session_root_path.parent_path() != workspace.cache_root_path ||
      workspace.session_root_path.filename() != "current") {
    return std::unexpected(
        DangerousCleanupPath(workspace.session_root_path));
  }

  return {};
}

std::expected<void, CacheError> ValidateCacheDirectory(const std::filesystem::path& path) {
  std::error_code error_code;
  const std::filesystem::file_status status = std::filesystem::symlink_status(path, error_code);
  if (error_code == std::errc::no_such_file_or_directory) {
    return {};
  }

  if (error_code) {
    return std::unexpected(OtherFilesystemError(path, error_code));
  }

  if (std::filesystem::is_symlink(status)) {
    return std::unexpected(SymlinkDetected(path));
  }

  if (std::filesystem::exists(status) && !std::filesystem::is_directory(status)) {
    return std::unexpected(PathIsNotDirectory(path));
  }

  return {};
}

std::expected<void, CacheError> ValidateSourceTree(const std::filesystem::path& source_project_path) {
  std::error_code error_code;
  std::filesystem::recursive_directory_iterator iterator(source_project_path, error_code);
  if (error_code) {
    return std::unexpected(
        FailedToReadSourceFile(source_project_path, error_code));
  }

  const std::filesystem::recursive_directory_iterator end;
  while (iterator != end) {
    const std::filesystem::path entry_path = iterator->path();
    const std::filesystem::file_status status = iterator->symlink_status(error_code);
    if (error_code) {
      return std::unexpected(FailedToReadSourceFile(entry_path, error_code));
    }

    if (std::filesystem::is_symlink(status)) {
      return std::unexpected(SymlinkDetected(entry_path));
    }

    iterator.increment(error_code);
    if (error_code) {
      return std::unexpected(FailedToReadSourceFile(entry_path, error_code));
    }
  }

  return {};
}

std::expected<void, CacheError> RemoveOldSession(const std::filesystem::path& session_root_path) {
  std::error_code error_code;
  std::filesystem::remove_all(session_root_path, error_code);
  if (error_code) {
    return std::unexpected(
        FailedToRemoveOldCache(session_root_path, error_code));
  }

  return {};
}

std::expected<void, CacheError> CreateWorkspaceDirectories(const Workspace& workspace) {
  const std::filesystem::path directories[] = {
      workspace.project_container_path, workspace.build_path,
      workspace.metadata_path, workspace.logs_path};

  for (const std::filesystem::path& directory : directories) {
    std::error_code error_code;
    std::filesystem::create_directories(directory, error_code);
    if (error_code) {
      return std::unexpected(FailedToCreateDirectory(directory, error_code));
    }
  }

  return {};
}

std::expected<void, CacheError> CopySourceProject(const Workspace& workspace) {
  std::error_code error_code;
  std::filesystem::copy(workspace.source_project_path, workspace.cached_project_path,
                        std::filesystem::copy_options::recursive, error_code);
  if (error_code) {
    return std::unexpected(
        FailedToCopyFile(workspace.source_project_path, error_code));
  }

  return {};
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

std::expected<Workspace, CacheError> WorkspaceCache::PrepareWorkspace(
    const std::filesystem::path& source_project_path) const {
  const auto workspace_result = CalculateWorkspace(source_project_path);
  if (!workspace_result) {
    return std::unexpected(workspace_result.error());
  }

  const Workspace& workspace = *workspace_result;

  if (const auto result = ValidateWorkspaceRelationships(workspace);
      !result) {
    return std::unexpected(result.error());
  }

  if (const auto result = ValidateCleanupTarget(workspace);
      !result) {
    return std::unexpected(result.error());
  }

  if (const auto result = ValidateCacheDirectory(workspace.cache_root_path);
      !result) {
    return std::unexpected(result.error());
  }

  if (const auto result = ValidateCacheDirectory(workspace.session_root_path);
      !result) {
    return std::unexpected(result.error());
  }

  if (const auto result = ValidateSourceTree(workspace.source_project_path);
      !result) {
    return std::unexpected(result.error());
  }

  if (const auto result = RemoveOldSession(workspace.session_root_path);
      !result) {
    return std::unexpected(result.error());
  }

  if (const auto result = CreateWorkspaceDirectories(workspace);
      !result) {
    return std::unexpected(result.error());
  }

  if (const auto result = CopySourceProject(workspace);
      !result) {
    std::error_code cleanup_error;
    std::filesystem::remove_all(workspace.session_root_path, cleanup_error);
    return std::unexpected(result.error());
  }

  return workspace;
}

}  // namespace cpp_defense
