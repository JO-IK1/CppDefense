#pragma once

#include <expected>
#include <filesystem>

#include "cpp_defense/core/cache_error.hpp"
#include "cpp_defense/core/workspace.hpp"

namespace cpp_defense {

class WorkspaceCache {
 public:
  explicit WorkspaceCache(std::filesystem::path cpp_defense_root_path);

  std::expected<Workspace, CacheError> CalculateWorkspace(
      const std::filesystem::path& source_project_path) const;

  std::expected<Workspace, CacheError> PrepareWorkspace(
      const std::filesystem::path& source_project_path) const;

 private:
  std::filesystem::path cpp_defense_root_path_;
};

}  // namespace cpp_defense
