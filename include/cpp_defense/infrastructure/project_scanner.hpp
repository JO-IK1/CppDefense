#pragma once

#include <expected>
#include <filesystem>
#include <vector>

#include "cpp_defense/core/scan_error.hpp"

namespace cpp_defense {

using SourceFilePaths = std::vector<std::filesystem::path>;

class ProjectScanner {
 public:
  std::expected<SourceFilePaths, ScanError> FindSourceFiles(
      const std::filesystem::path& cached_project_path) const;
};

}  // namespace cpp_defense
