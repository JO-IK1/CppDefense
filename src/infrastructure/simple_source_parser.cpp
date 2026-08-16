#include "cpp_defense/infrastructure/simple_source_parser.hpp"

#include <expected>
#include <filesystem>
#include <string_view>
#include <utility>

#include "source_parser/source_entity_finder.hpp"
#include "source_parser/source_lexer.hpp"
#include "source_parser/source_structure.hpp"

namespace cpp_defense {

std::expected<CodeEntities, ParseError> SimpleSourceParser::Parse(
    std::string_view source, const std::filesystem::path& file_path) const {
  auto lex_result = source_parser_internal::LexSource(source, file_path);

  if (!lex_result) {
    return std::unexpected(std::move(lex_result.error()));
  }

  auto structure = source_parser_internal::BuildSourceStructure(
      lex_result->sanitized_source, lex_result->line_starts, file_path);

  if (!structure) {
    return std::unexpected(std::move(structure.error()));
  }

  return source_parser_internal::FindSourceEntities(
      lex_result->sanitized_source, *structure, lex_result->line_starts,
      file_path);
}

}  // namespace cpp_defense
