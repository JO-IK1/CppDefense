#pragma once

#include <expected>
#include <filesystem>

#include "cpp_defense/core/error.hpp"
#include "cpp_defense/core/workspace.hpp"
#include "cpp_defense/infrastructure/workspace_cache.hpp"

namespace cpp_defense {

class DefenseService {
 public:
  explicit DefenseService(std::filesystem::path cpp_defense_root_path);

  std::expected<Workspace, CacheError> PrepareWorkspace(
      const std::filesystem::path& source_project_path) const;

 private:
  WorkspaceCache workspace_cache_;
};

}  // namespace cpp_defense
