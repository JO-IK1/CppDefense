#pragma once

#include <expected>
#include <filesystem>

#include "cpp_defense/core/check_workspace_error.hpp"
#include "cpp_defense/core/workspace.hpp"

namespace cpp_defense {

struct CheckWorkspacePaths {
  std::filesystem::path root_path;
  std::filesystem::path project_path;
  std::filesystem::path build_path;
};

class CheckWorkspace {
 public:
  std::expected<CheckWorkspacePaths, CheckWorkspaceError> Prepare(
      const Workspace& workspace) const;

  void Cleanup(const CheckWorkspacePaths& paths) const noexcept;
};

}  // namespace cpp_defense
