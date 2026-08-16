#pragma once

#include <expected>
#include <filesystem>
#include <string_view>

#include "cpp_defense/core/parse_error.hpp"
#include "source_parser_common.hpp"

namespace cpp_defense::source_parser_internal {

std::expected<LexResult, ParseError> LexSource(
    std::string_view source, const std::filesystem::path& file_path);

}  // namespace cpp_defense::source_parser_internal
