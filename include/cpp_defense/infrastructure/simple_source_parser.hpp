#pragma once

#include <expected>
#include <filesystem>
#include <string_view>
#include <vector>

#include "cpp_defense/core/code_entity_info.hpp"
#include "cpp_defense/core/parse_error.hpp"

namespace cpp_defense {

using CodeEntities = std::vector<CodeEntityInfo>;

class SimpleSourceParser {
 public:
  std::expected<CodeEntities, ParseError> Parse(
      std::string_view source, const std::filesystem::path& file_path) const;
};

}  // namespace cpp_defense