#include "source_type_parser.hpp"

#include <cstddef>
#include <optional>
#include <string>
#include <string_view>

namespace cpp_defense::source_parser_internal {
namespace {

bool IsTokenBoundary(std::string_view source, std::size_t offset) {
  return offset >= source.size() || !IsIdentifierCharacter(source[offset]);
}

struct TypeKeywordMatch {
  CodeEntityType type;
  std::size_t declaration_start = 0;
  std::size_t name_search_start = 0;
};

std::optional<TypeKeywordMatch> FindLastTypeKeyword(
    std::string_view declaration) {
  std::optional<TypeKeywordMatch> result;
  std::string_view previous_identifier;
  std::size_t previous_token_start = kNoOffset;

  for (std::size_t offset = 0; offset < declaration.size();) {
    if (!IsIdentifierStart(declaration[offset])) {
      ++offset;
      continue;
    }

    const std::size_t token_start = offset;
    ++offset;

    while (offset < declaration.size() &&
           IsIdentifierCharacter(declaration[offset])) {
      ++offset;
    }

    const std::string_view token =
        declaration.substr(token_start, offset - token_start);

    if (token == "class") {
      if (previous_identifier == "enum" &&
          previous_token_start != kNoOffset) {
        result = TypeKeywordMatch{
            .type = CodeEntityType::kEnumClass,
            .declaration_start = previous_token_start,
            .name_search_start = offset,
        };
      } else {
        result = TypeKeywordMatch{
            .type = CodeEntityType::kClass,
            .declaration_start = token_start,
            .name_search_start = offset,
        };
      }
    } else if (token == "struct") {
      if (previous_identifier == "enum" &&
          previous_token_start != kNoOffset) {
        result = TypeKeywordMatch{
            .type = CodeEntityType::kEnumClass,
            .declaration_start = previous_token_start,
            .name_search_start = offset,
        };
      } else {
        result = TypeKeywordMatch{
            .type = CodeEntityType::kStruct,
            .declaration_start = token_start,
            .name_search_start = offset,
        };
      }
    }

    previous_identifier = token;
    previous_token_start = token_start;
  }

  return result;
}


}  // namespace

std::optional<EntityCandidate> TryFindTypeCandidate(
    std::string_view source,
    const StructureInfo& structure,
    std::size_t opening_brace) {
  const std::size_t boundary = FindPreviousBoundary(source, opening_brace);
  const std::string_view declaration =
      source.substr(boundary, opening_brace - boundary);

  const auto keyword = FindLastTypeKeyword(declaration);

  if (!keyword) {
    return std::nullopt;
  }

  const CodeEntityType type = keyword->type;
  const std::size_t declaration_start = keyword->declaration_start;

  std::size_t name_start = keyword->name_search_start;
  name_start = SkipWhitespaceForward(declaration, name_start,
                                     declaration.size());

  if (name_start >= declaration.size() ||
      !IsIdentifierStart(declaration[name_start])) {
    return std::nullopt;
  }

  std::size_t name_end = name_start + 1;

  while (name_end < declaration.size() &&
         IsIdentifierCharacter(declaration[name_end])) {
    ++name_end;
  }

  if (!IsTokenBoundary(declaration, name_end)) {
    return std::nullopt;
  }

  const std::string_view suffix = declaration.substr(name_end);

  if (suffix.find('(') != std::string_view::npos ||
      suffix.find(')') != std::string_view::npos) {
    return std::nullopt;
  }

  const std::size_t closing_brace =
      structure.matching_braces[opening_brace];

  if (closing_brace == kNoOffset) {
    return std::nullopt;
  }

  std::size_t end_offset = closing_brace + 1;
  std::size_t semicolon_offset = end_offset;

  while (semicolon_offset < source.size() &&
         IsSpace(source[semicolon_offset])) {
    ++semicolon_offset;
  }

  if (semicolon_offset < source.size() &&
      source[semicolon_offset] == ';') {
    end_offset = semicolon_offset + 1;
  }

  const std::string_view name =
      declaration.substr(name_start, name_end - name_start);

  return EntityCandidate{
      .type = type,
      .name = std::string(name),
      .start_offset = boundary + declaration_start,
      .opening_brace = opening_brace,
      .closing_brace = closing_brace,
      .end_offset = end_offset,
  };
}

}  // namespace cpp_defense::source_parser_internal
