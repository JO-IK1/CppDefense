#pragma once

#include <expected>
#include <filesystem>

#include "cpp_defense/application/prepared_project.hpp"
#include "cpp_defense/application/project_preparation_error.hpp"
#include "cpp_defense/infrastructure/project_scanner.hpp"
#include "cpp_defense/infrastructure/workspace_cache.hpp"

namespace cpp_defense {

class DefenseService {
 public:
  explicit DefenseService(std::filesystem::path cpp_defense_root_path);

  std::expected<PreparedProject, ProjectPreparationError> PrepareProject(
      const std::filesystem::path& source_project_path) const;

 private:
  WorkspaceCache workspace_cache_;
  ProjectScanner project_scanner_;
};

}  // namespace cpp_defense
