#include "cpp_defense/application/defense_service.hpp"

#include <utility>

namespace cpp_defense {

DefenseService::DefenseService(std::filesystem::path cpp_defense_root_path)
    : workspace_cache_(std::move(cpp_defense_root_path)) {}

std::expected<Workspace, CacheError> DefenseService::PrepareWorkspace(
    const std::filesystem::path& source_project_path) const {
  return workspace_cache_.PrepareWorkspace(source_project_path);
}

}  // namespace cpp_defense
