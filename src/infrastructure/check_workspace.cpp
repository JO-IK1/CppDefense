#include "cpp_defense/infrastructure/check_workspace.hpp"

#include <system_error>

namespace cpp_defense {

std::expected<CheckWorkspacePaths, CheckWorkspaceError>
CheckWorkspace::Prepare(const Workspace& workspace) const {
  CheckWorkspacePaths paths;
  paths.root_path = workspace.session_root_path / "check";
  const std::filesystem::path project_container = paths.root_path / "project";
  paths.project_path = project_container / workspace.cached_project_path.filename();
  paths.build_path = paths.root_path / "build";

  std::error_code error_code;
  std::filesystem::remove_all(paths.root_path, error_code);
  if (error_code) {
    return std::unexpected(CheckWorkspaceError(
        CheckWorkspaceErrorType::kFailedToRemoveOldWorkspace,
        "Failed to remove previous check workspace",
        paths.root_path, error_code));
  }

  std::filesystem::create_directories(project_container, error_code);
  if (error_code) {
    return std::unexpected(CheckWorkspaceError(
        CheckWorkspaceErrorType::kFailedToCreateDirectory,
        "Failed to create check project directory",
        project_container, error_code));
  }

  std::filesystem::create_directories(paths.build_path, error_code);
  if (error_code) {
    return std::unexpected(CheckWorkspaceError(
        CheckWorkspaceErrorType::kFailedToCreateDirectory,
        "Failed to create check build directory",
        paths.build_path, error_code));
  }

  std::filesystem::copy(
      workspace.cached_project_path,
      paths.project_path,
      std::filesystem::copy_options::recursive,
      error_code);
  if (error_code) {
    return std::unexpected(CheckWorkspaceError(
        CheckWorkspaceErrorType::kFailedToCopyProject,
        "Failed to copy masked project into check workspace",
        workspace.cached_project_path, error_code));
  }

  return paths;
}

void CheckWorkspace::Cleanup(const CheckWorkspacePaths& paths) const noexcept {
  std::error_code error_code;
  std::filesystem::remove_all(paths.root_path, error_code);
}

}  // namespace cpp_defense
