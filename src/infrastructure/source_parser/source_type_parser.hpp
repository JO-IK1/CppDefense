#pragma once

#include <cstddef>
#include <optional>
#include <string_view>

#include "source_parser_common.hpp"

namespace cpp_defense::source_parser_internal {

std::optional<EntityCandidate> TryFindTypeCandidate(
    std::string_view source,
    const StructureInfo& structure,
    std::size_t opening_brace);

}  // namespace cpp_defense::source_parser_internal
