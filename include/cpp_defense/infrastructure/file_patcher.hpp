#pragma once

#include <expected>
#include <filesystem>
#include <string_view>

#include "cpp_defense/core/code_entity_info.hpp"
#include "cpp_defense/core/file_patcher_error.hpp"

namespace cpp_defense {

class FilePatcher {
 public:
  std::expected<std::filesystem::path, FilePatcherError> Patch(
      const CodeEntityInfo& entity,
      const std::filesystem::path& cached_project_path,
      const std::filesystem::path& check_project_path,
      std::string_view replacement) const;
};

}  // namespace cpp_defense
