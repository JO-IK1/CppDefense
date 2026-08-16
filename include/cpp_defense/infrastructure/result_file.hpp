#pragma once

#include <expected>
#include <filesystem>
#include <string>

#include "cpp_defense/core/code_entity_info.hpp"
#include "cpp_defense/core/result_error.hpp"

namespace cpp_defense {

class ResultFile {
 public:
  std::expected<void, ResultFileError> Create(
      const CodeEntityInfo& entity,
      const std::filesystem::path& result_path) const;

  std::expected<std::string, ResultFileError> Read(
      const std::filesystem::path& result_path) const;
};

}  // namespace cpp_defense
