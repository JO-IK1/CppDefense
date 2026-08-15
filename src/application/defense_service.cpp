#include "cpp_defense/application/defense_service.hpp"

#include <utility>

namespace cpp_defense {

DefenseService::DefenseService(std::filesystem::path cpp_defense_root_path)
    : workspace_cache_(std::move(cpp_defense_root_path)) {}

std::expected<PreparedProject, ProjectPreparationError> DefenseService::PrepareProject(
    const std::filesystem::path& source_project_path) const {
  auto workspace = workspace_cache_.PrepareWorkspace(source_project_path);

  if (!workspace) {
    return std::unexpected(
        ProjectPreparationError{workspace.error()});
  }

  auto source_files =
      project_scanner_.FindSourceFiles(workspace->cached_project_path);

  if (!source_files) {
    return std::unexpected(
        ProjectPreparationError{source_files.error()});
  }

  return PreparedProject{
      .workspace = std::move(*workspace),
      .source_files = std::move(*source_files),
  };
}

}  // namespace cpp_defense
