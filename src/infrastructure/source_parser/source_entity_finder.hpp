#pragma once

#include <filesystem>
#include <string_view>
#include <vector>

#include "cpp_defense/core/code_entity_info.hpp"
#include "source_parser_common.hpp"

namespace cpp_defense::source_parser_internal {

std::vector<CodeEntityInfo> FindSourceEntities(
    std::string_view source,
    const StructureInfo& structure,
    const std::vector<std::size_t>& line_starts,
    const std::filesystem::path& file_path);

}  // namespace cpp_defense::source_parser_internal
