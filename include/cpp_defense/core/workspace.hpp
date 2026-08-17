#pragma once

#include <filesystem>

namespace cpp_defense {

struct Workspace {
  std::filesystem::path cpp_defense_root_path;
  std::filesystem::path source_project_path;
  std::filesystem::path cache_root_path;
  std::filesystem::path session_root_path;
  std::filesystem::path project_container_path;
  std::filesystem::path cached_project_path;
  std::filesystem::path build_path;
  std::filesystem::path metadata_path;
  std::filesystem::path logs_path;
  std::filesystem::path result_path;
  std::filesystem::path session_data_path;
  std::filesystem::path original_fragment_path;
  std::filesystem::path defense_result_path;
};

}  // namespace cpp_defense
