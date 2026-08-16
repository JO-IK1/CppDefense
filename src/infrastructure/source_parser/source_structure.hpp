#pragma once

#include <expected>
#include <filesystem>
#include <string_view>
#include <vector>

#include "cpp_defense/core/parse_error.hpp"
#include "source_parser_common.hpp"

namespace cpp_defense::source_parser_internal {

std::expected<StructureInfo, ParseError> BuildSourceStructure(
    std::string_view source,
    const std::vector<std::size_t>& line_starts,
    const std::filesystem::path& file_path);

}  // namespace cpp_defense::source_parser_internal
