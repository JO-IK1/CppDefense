#pragma once

#include <filesystem>
#include <vector>

#include "cpp_defense/core/workspace.hpp"

namespace cpp_defense {

struct PreparedProject {
  Workspace workspace;
  std::vector<std::filesystem::path> source_files;
};

}  // namespace cpp_defense
