#pragma once

#include <expected>
#include <filesystem>
#include <string>
#include <vector>

#include "cpp_defense/core/scan_error.hpp"

namespace cpp_defense {

using SourceFilePaths = std::vector<std::filesystem::path>;

struct ProjectScannerOptions {
  std::vector<std::string> excluded_directory_names{
      ".git",
      ".idea",
      ".vscode",
      "build",
      "cache",
      "cmake-build-debug",
      "cmake-build-release",
      "test",
      "tests",
  };
};

class ProjectScanner {
 public:
  explicit ProjectScanner(ProjectScannerOptions options = {});

  std::expected<SourceFilePaths, ScanError> FindSourceFiles(
      const std::filesystem::path& cached_project_path) const;

 private:
  ProjectScannerOptions options_;
};

}  // namespace cpp_defense
