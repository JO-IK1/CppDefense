#include "internal.hpp"

#include <algorithm>
#include <array>
#include <cstddef>
#include <optional>
#include <string>
#include <string_view>

namespace cpp_defense::source_parser_detail {
namespace {

constexpr std::array<std::string_view, 10> kRejectedFunctionNames{
    "if",       "for",      "while",    "switch",  "catch",
    "sizeof",   "alignof",  "decltype", "noexcept", "static_assert",
};

constexpr std::array<std::string_view, 3> kAccessSpecifiers{
    "public:",
    "protected:",
    "private:",
};

bool IsRejectedFunctionName(std::string_view name) {
  return std::find(kRejectedFunctionNames.begin(),
                   kRejectedFunctionNames.end(),
                   name) != kRejectedFunctionNames.end();
}

bool IsValidIdentifier(std::string_view value) {
  if (value.empty() || !IsIdentifierStart(value.front())) {
    return false;
  }

  return std::all_of(value.begin() + 1, value.end(), [](char character) {
    return IsIdentifierCharacter(character);
  });
}

bool IsValidQualifiedName(std::string_view name) {
  if (name.empty()) {
    return false;
  }

  std::size_t start = 0;

  while (start < name.size()) {
    const std::size_t separator = name.find("::", start);
    const std::size_t end =
        separator == std::string_view::npos ? name.size() : separator;

    std::string_view part = name.substr(start, end - start);

    if (!part.empty() && part.front() == '~') {
      part.remove_prefix(1);
    }

    if (!IsValidIdentifier(part)) {
      return false;
    }

    if (separator == std::string_view::npos) {
      break;
    }

    start = separator + 2;
  }

  return true;
}

std::size_t SkipWhitespaceBackward(std::string_view source,
                                   std::size_t offset) {
  while (offset > 0 && IsSpace(source[offset - 1])) {
    --offset;
  }

  return offset;
}

bool IsTokenAt(std::string_view source, std::size_t position,
               std::string_view token) {
  if (position + token.size() > source.size() ||
      source.substr(position, token.size()) != token) {
    return false;
  }

  const bool valid_left =
      position == 0 || !IsIdentifierCharacter(source[position - 1]);
  const std::size_t end = position + token.size();
  const bool valid_right =
      end == source.size() || !IsIdentifierCharacter(source[end]);

  return valid_left && valid_right;
}

std::size_t FindLastToken(std::string_view source, std::size_t begin,
                          std::size_t end, std::string_view token) {
  if (begin >= end || token.empty()) {
    return std::string_view::npos;
  }

  std::size_t position = source.rfind(token, end - 1);

  while (position != std::string_view::npos && position >= begin) {
    if (position + token.size() <= end &&
        IsTokenAt(source, position, token)) {
      return position;
    }

    if (position == 0) {
      break;
    }

    position = source.rfind(token, position - 1);
  }

  return std::string_view::npos;
}

std::size_t FindQualifierStart(std::string_view source,
                               std::size_t name_start,
                               std::size_t boundary) {
  std::size_t begin = SkipWhitespaceBackward(source, name_start);

  while (begin >= boundary + 2 && source[begin - 1] == ':' &&
         source[begin - 2] == ':') {
    begin -= 2;
    begin = SkipWhitespaceBackward(source, begin);

    std::size_t identifier_begin = begin;

    while (identifier_begin > boundary &&
           IsIdentifierCharacter(source[identifier_begin - 1])) {
      --identifier_begin;
    }

    if (identifier_begin == begin ||
        !IsIdentifierStart(source[identifier_begin])) {
      break;
    }

    begin = identifier_begin;
    begin = SkipWhitespaceBackward(source, begin);
  }

  return begin;
}

std::string ExtractFunctionName(std::string_view source,
                                std::size_t opening_parenthesis,
                                std::size_t boundary) {
  if (opening_parenthesis == 0) {
    return {};
  }

  const std::size_t end =
      SkipWhitespaceBackward(source, opening_parenthesis);

  std::size_t direct_begin = end;
  while (direct_begin > boundary &&
         IsIdentifierCharacter(source[direct_begin - 1])) {
    --direct_begin;
  }

  const std::string_view direct_identifier =
      source.substr(direct_begin, end - direct_begin);

  if (IsRejectedFunctionName(direct_identifier)) {
    return std::string(direct_identifier);
  }

  const std::size_t operator_position =
      FindLastToken(source, boundary, end, "operator");

  if (operator_position != std::string_view::npos) {
    const std::size_t begin =
        FindQualifierStart(source, operator_position, boundary);
    std::string_view name = source.substr(begin, end - begin);

    while (!name.empty() && IsSpace(name.front())) {
      name.remove_prefix(1);
    }
    while (!name.empty() && IsSpace(name.back())) {
      name.remove_suffix(1);
    }

    return std::string(name);
  }

  std::size_t begin = end;

  while (begin > boundary) {
    const char character = source[begin - 1];

    if (IsIdentifierCharacter(character) || character == ':' ||
        character == '~') {
      --begin;
      continue;
    }

    break;
  }

  return std::string(source.substr(begin, end - begin));
}

std::string_view GetUnqualifiedFunctionName(std::string_view name) {
  const std::size_t operator_position = name.find("operator");

  if (operator_position != std::string_view::npos) {
    return name.substr(operator_position);
  }

  const std::size_t separator = name.rfind("::");
  return separator == std::string_view::npos ? name
                                              : name.substr(separator + 2);
}

bool IsValidOperatorName(std::string_view name) {
  const std::size_t operator_position = name.find("operator");

  if (operator_position == std::string_view::npos) {
    return false;
  }

  const std::string_view suffix = name.substr(operator_position + 8);

  if (suffix.empty()) {
    return false;
  }

  if (operator_position == 0) {
    return true;
  }

  std::string_view qualifier = name.substr(0, operator_position);

  while (!qualifier.empty() && IsSpace(qualifier.back())) {
    qualifier.remove_suffix(1);
  }

  if (!qualifier.ends_with("::")) {
    return false;
  }

  qualifier.remove_suffix(2);
  return IsValidQualifiedName(qualifier);
}

bool IsValidFunctionName(std::string_view name) {
  return IsValidQualifiedName(name) || IsValidOperatorName(name);
}

bool ContainsSingleColon(std::string_view value) {
  for (std::size_t offset = 0; offset < value.size(); ++offset) {
    if (value[offset] != ':') {
      continue;
    }

    const bool previous_is_colon = offset > 0 && value[offset - 1] == ':';
    const bool next_is_colon =
        offset + 1 < value.size() && value[offset + 1] == ':';

    if (!previous_is_colon && !next_is_colon) {
      return true;
    }
  }

  return false;
}

bool IsSupportedFunctionSuffix(std::string_view suffix) {
  if (suffix.find(';') != std::string_view::npos ||
      suffix.find('=') != std::string_view::npos ||
      suffix.find('{') != std::string_view::npos ||
      suffix.find('}') != std::string_view::npos) {
    return false;
  }

  return !ContainsSingleColon(suffix);
}

std::size_t FindFunctionStart(std::string_view source,
                              std::size_t opening_parenthesis) {
  const std::size_t boundary =
      FindPreviousBoundary(source, opening_parenthesis);
  std::size_t start = boundary;

  const std::string_view prefix =
      source.substr(boundary, opening_parenthesis - boundary);

  for (const std::string_view access_specifier : kAccessSpecifiers) {
    const std::size_t position = prefix.rfind(access_specifier);

    if (position != std::string_view::npos) {
      start = std::max(start, boundary + position + access_specifier.size());
    }
  }

  return SkipWhitespaceForward(source, start, opening_parenthesis);
}

std::string_view PreviousIdentifier(std::string_view source,
                                    std::size_t offset,
                                    std::size_t boundary) {
  offset = SkipWhitespaceBackward(source, offset);
  std::size_t end = offset;

  while (offset > boundary &&
         IsIdentifierCharacter(source[offset - 1])) {
    --offset;
  }

  if (offset == end) {
    return {};
  }

  return source.substr(offset, end - offset);
}

std::size_t FindConstructorInitializerColon(std::string_view source,
                                            std::size_t boundary,
                                            std::size_t opening_brace) {
  for (std::size_t offset = boundary; offset < opening_brace; ++offset) {
    if (source[offset] != ':') {
      continue;
    }

    const bool previous_is_colon = offset > boundary &&
                                   source[offset - 1] == ':';
    const bool next_is_colon = offset + 1 < opening_brace &&
                               source[offset + 1] == ':';

    if (previous_is_colon || next_is_colon) {
      continue;
    }

    const std::string_view previous =
        PreviousIdentifier(source, offset, boundary);

    if (previous == "public" || previous == "protected" ||
        previous == "private") {
      continue;
    }

    return offset;
  }

  return kNoOffset;
}

bool ContainsToken(std::string_view source, std::size_t begin,
                   std::size_t end, std::string_view token) {
  return FindLastToken(source, begin, end, token) != std::string_view::npos;
}

}  // namespace

std::optional<EntityCandidate> TryFindFunctionCandidate(
    std::string_view source,
    const StructureInfo& structure,
    std::size_t opening_brace) {
  if (opening_brace == 0) {
    return std::nullopt;
  }

  const std::size_t boundary = FindPreviousBoundary(source, opening_brace);
  const std::size_t initializer_colon =
      FindConstructorInitializerColon(source, boundary, opening_brace);
  std::size_t search_position = initializer_colon == kNoOffset
                                    ? opening_brace
                                    : initializer_colon;

  while (search_position > boundary) {
    const std::size_t closing_parenthesis =
        source.rfind(')', search_position - 1);

    if (closing_parenthesis == std::string_view::npos ||
        closing_parenthesis < boundary) {
      break;
    }

    const std::size_t opening_parenthesis =
        structure.matching_parentheses[closing_parenthesis];

    if (opening_parenthesis == kNoOffset ||
        opening_parenthesis < boundary) {
      search_position = closing_parenthesis;
      continue;
    }

    const std::string name =
        ExtractFunctionName(source, opening_parenthesis, boundary);

    if (name.empty()) {
      search_position = opening_parenthesis;
      continue;
    }

    std::string_view short_name = GetUnqualifiedFunctionName(name);

    if (!short_name.empty() && short_name.front() == '~') {
      short_name.remove_prefix(1);
    }

    if (!IsValidFunctionName(name) ||
        IsRejectedFunctionName(short_name)) {
      search_position = opening_parenthesis;
      continue;
    }

    const std::size_t suffix_end = initializer_colon == kNoOffset
                                       ? opening_brace
                                       : initializer_colon;
    const std::string_view suffix = source.substr(
        closing_parenthesis + 1,
        suffix_end - closing_parenthesis - 1);

    if (!IsSupportedFunctionSuffix(suffix)) {
      search_position = opening_parenthesis;
      continue;
    }

    const std::size_t closing_brace =
        structure.matching_braces[opening_brace];

    if (closing_brace == kNoOffset) {
      return std::nullopt;
    }

    return EntityCandidate{
        .type = CodeEntityType::kFunction,
        .name = name,
        .start_offset = FindFunctionStart(source, opening_parenthesis),
        .opening_brace = opening_brace,
        .closing_brace = closing_brace,
        .end_offset = closing_brace + 1,
        .is_friend = ContainsToken(source, boundary, opening_parenthesis,
                                   "friend"),
    };
  }

  return std::nullopt;
}

}  // namespace cpp_defense::source_parser_detail
