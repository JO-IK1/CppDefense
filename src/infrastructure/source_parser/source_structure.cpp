#include "source_structure.hpp"

#include <cstddef>
#include <expected>
#include <filesystem>
#include <string_view>
#include <vector>

namespace cpp_defense::source_parser_internal {

std::expected<StructureInfo, ParseError> BuildSourceStructure(
    std::string_view source,
    const std::vector<std::size_t>& line_starts,
    const std::filesystem::path& file_path) {
  StructureInfo structure{
      .matching_parentheses =
          std::vector<std::size_t>(source.size(), kNoOffset),
      .matching_braces = std::vector<std::size_t>(source.size(), kNoOffset),
  };

  std::vector<std::size_t> parenthesis_stack;
  std::vector<std::size_t> brace_stack;

  for (std::size_t offset = 0; offset < source.size(); ++offset) {
    switch (source[offset]) {
      case '(':
        parenthesis_stack.push_back(offset);
        break;

      case ')': {
        if (parenthesis_stack.empty()) {
          return std::unexpected(UnmatchedClosingParenthesis(
              file_path, LineFromOffset(line_starts, offset), offset));
        }

        const std::size_t opening_offset = parenthesis_stack.back();
        parenthesis_stack.pop_back();

        structure.matching_parentheses[opening_offset] = offset;
        structure.matching_parentheses[offset] = opening_offset;
        break;
      }

      case '{':
        brace_stack.push_back(offset);
        break;

      case '}': {
        if (brace_stack.empty()) {
          return std::unexpected(UnmatchedClosingBrace(
              file_path, LineFromOffset(line_starts, offset), offset));
        }

        const std::size_t opening_offset = brace_stack.back();
        brace_stack.pop_back();

        structure.matching_braces[opening_offset] = offset;
        structure.matching_braces[offset] = opening_offset;
        break;
      }

      default:
        break;
    }
  }

  if (!parenthesis_stack.empty()) {
    const std::size_t offset = parenthesis_stack.back();
    return std::unexpected(UnmatchedOpeningParenthesis(
        file_path, LineFromOffset(line_starts, offset), offset));
  }

  if (!brace_stack.empty()) {
    const std::size_t offset = brace_stack.back();
    return std::unexpected(UnmatchedOpeningBrace(
        file_path, LineFromOffset(line_starts, offset), offset));
  }

  return structure;
}

}  // namespace cpp_defense::source_parser_internal
