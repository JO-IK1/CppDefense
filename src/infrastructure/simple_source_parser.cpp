#include "cpp_defense/infrastructure/simple_source_parser.hpp"

#include <algorithm>
#include <array>
#include <cctype>
#include <cstddef>
#include <expected>
#include <filesystem>
#include <limits>
#include <optional>
#include <string>
#include <string_view>
#include <utility>
#include <vector>

namespace cpp_defense {
namespace {

constexpr std::size_t kNoOffset = std::numeric_limits<std::size_t>::max();

constexpr std::array<std::string_view, 10> kRejectedFunctionNames{
    "if",       "for",      "while",    "switch",  "catch",
    "sizeof",   "alignof",  "decltype", "noexcept", "static_assert",
};

constexpr std::array<std::string_view, 3> kAccessSpecifiers{
    "public:",
    "protected:",
    "private:",
};

enum class LexerState {
  kCode,
  kLineComment,
  kBlockComment,
  kStringLiteral,
  kCharacterLiteral,
  kRawStringLiteral,
  kPreprocessorDirective,
};

struct LexResult {
  std::string sanitized_source;
  std::vector<std::size_t> line_starts;
};

struct StructureInfo {
  std::vector<std::size_t> matching_parentheses;
  std::vector<std::size_t> matching_braces;
};

struct EntityCandidate {
  CodeEntityType type;
  std::string name;
  std::size_t start_offset = 0;
  std::size_t opening_brace = 0;
  std::size_t closing_brace = 0;
  std::size_t end_offset = 0;
  bool is_friend = false;
};

bool IsSpace(char character) {
  return std::isspace(static_cast<unsigned char>(character)) != 0;
}

bool IsIdentifierStart(char character) {
  const unsigned char value = static_cast<unsigned char>(character);
  return std::isalpha(value) != 0 || character == '_';
}

bool IsIdentifierCharacter(char character) {
  const unsigned char value = static_cast<unsigned char>(character);
  return std::isalnum(value) != 0 || character == '_';
}

void MaskCharacter(std::string& source, std::size_t offset) {
  if (source[offset] != '\n' && source[offset] != '\r') {
    source[offset] = ' ';
  }
}

std::size_t LineFromOffset(const std::vector<std::size_t>& line_starts,
                           std::size_t offset) {
  const auto iterator =
      std::upper_bound(line_starts.begin(), line_starts.end(), offset);

  return static_cast<std::size_t>(
      std::distance(line_starts.begin(), iterator));
}

std::vector<std::size_t> BuildLineStarts(std::string_view source) {
  std::vector<std::size_t> line_starts{0};

  for (std::size_t offset = 0; offset < source.size(); ++offset) {
    if (source[offset] == '\n') {
      line_starts.push_back(offset + 1);
    }
  }

  return line_starts;
}

bool IsRawDelimiterCharacter(char character) {
  const unsigned char value = static_cast<unsigned char>(character);

  return std::isspace(value) == 0 && character != '(' && character != ')' &&
         character != '\\';
}

bool FindRawStringOpeningParenthesis(std::string_view source,
                                     std::size_t raw_start,
                                     std::size_t& opening_parenthesis) {
  if (raw_start + 1 >= source.size() || source[raw_start] != 'R' ||
      source[raw_start + 1] != '"') {
    return false;
  }

  const std::size_t delimiter_start = raw_start + 2;
  const std::size_t maximum_delimiter_end =
      std::min(source.size(), delimiter_start + 17);

  for (std::size_t offset = delimiter_start;
       offset < maximum_delimiter_end; ++offset) {
    if (source[offset] == '(') {
      opening_parenthesis = offset;
      return true;
    }

    if (!IsRawDelimiterCharacter(source[offset])) {
      return false;
    }
  }

  return false;
}

bool IsDirectiveContinued(std::string_view source,
                          std::size_t newline_offset) {
  if (newline_offset == 0) {
    return false;
  }

  if (source[newline_offset - 1] == '\\') {
    return true;
  }

  return newline_offset >= 2 && source[newline_offset - 1] == '\r' &&
         source[newline_offset - 2] == '\\';
}

std::expected<LexResult, ParseError> Lex(
    std::string_view source, const std::filesystem::path& file_path) {
  LexResult result{
      .sanitized_source = std::string(source),
      .line_starts = BuildLineStarts(source),
  };

  LexerState state = LexerState::kCode;
  std::size_t construct_start = 0;
  std::string raw_delimiter;
  bool at_line_start = true;

  for (std::size_t offset = 0; offset < source.size(); ++offset) {
    const char character = source[offset];

    switch (state) {
      case LexerState::kCode: {
        if (character == '\n') {
          at_line_start = true;
          continue;
        }

        if (at_line_start &&
            (character == ' ' || character == '\t' || character == '\r')) {
          continue;
        }

        if (at_line_start && character == '#') {
          state = LexerState::kPreprocessorDirective;
          MaskCharacter(result.sanitized_source, offset);
          at_line_start = false;
          continue;
        }

        at_line_start = false;

        if (character == '/' && offset + 1 < source.size() &&
            source[offset + 1] == '/') {
          state = LexerState::kLineComment;
          construct_start = offset;

          MaskCharacter(result.sanitized_source, offset);
          MaskCharacter(result.sanitized_source, offset + 1);
          ++offset;
          continue;
        }

        if (character == '/' && offset + 1 < source.size() &&
            source[offset + 1] == '*') {
          state = LexerState::kBlockComment;
          construct_start = offset;

          MaskCharacter(result.sanitized_source, offset);
          MaskCharacter(result.sanitized_source, offset + 1);
          ++offset;
          continue;
        }

        std::size_t raw_opening_parenthesis = 0;

        if (FindRawStringOpeningParenthesis(
                source, offset, raw_opening_parenthesis)) {
          state = LexerState::kRawStringLiteral;
          construct_start = offset;

          const std::size_t delimiter_start = offset + 2;
          raw_delimiter = std::string(source.substr(
              delimiter_start, raw_opening_parenthesis - delimiter_start));

          for (std::size_t current = offset;
               current <= raw_opening_parenthesis; ++current) {
            MaskCharacter(result.sanitized_source, current);
          }

          offset = raw_opening_parenthesis;
          continue;
        }

        if (character == '"') {
          state = LexerState::kStringLiteral;
          construct_start = offset;
          MaskCharacter(result.sanitized_source, offset);
          continue;
        }

        if (character == '\'') {
          state = LexerState::kCharacterLiteral;
          construct_start = offset;
          MaskCharacter(result.sanitized_source, offset);
          continue;
        }

        break;
      }

      case LexerState::kLineComment: {
        if (character == '\n') {
          state = LexerState::kCode;
          at_line_start = true;
          continue;
        }

        MaskCharacter(result.sanitized_source, offset);
        break;
      }

      case LexerState::kBlockComment: {
        if (character == '*' && offset + 1 < source.size() &&
            source[offset + 1] == '/') {
          MaskCharacter(result.sanitized_source, offset);
          MaskCharacter(result.sanitized_source, offset + 1);
          ++offset;
          state = LexerState::kCode;
          continue;
        }

        if (character == '\n') {
          at_line_start = true;
          continue;
        }

        MaskCharacter(result.sanitized_source, offset);
        break;
      }

      case LexerState::kStringLiteral: {
        if (character == '\n') {
          return std::unexpected(UnterminatedStringLiteral(
              file_path, LineFromOffset(result.line_starts, construct_start),
              construct_start));
        }

        MaskCharacter(result.sanitized_source, offset);

        if (character == '\\' && offset + 1 < source.size()) {
          if (source[offset + 1] == '\n') {
            ++offset;
            continue;
          }

          if (source[offset + 1] == '\r' && offset + 2 < source.size() &&
              source[offset + 2] == '\n') {
            offset += 2;
            continue;
          }

          MaskCharacter(result.sanitized_source, offset + 1);
          ++offset;
          continue;
        }

        if (character == '"') {
          state = LexerState::kCode;
        }

        break;
      }

      case LexerState::kCharacterLiteral: {
        if (character == '\n') {
          return std::unexpected(UnterminatedCharacterLiteral(
              file_path, LineFromOffset(result.line_starts, construct_start),
              construct_start));
        }

        MaskCharacter(result.sanitized_source, offset);

        if (character == '\\' && offset + 1 < source.size()) {
          if (source[offset + 1] == '\n') {
            ++offset;
            continue;
          }

          if (source[offset + 1] == '\r' && offset + 2 < source.size() &&
              source[offset + 2] == '\n') {
            offset += 2;
            continue;
          }

          MaskCharacter(result.sanitized_source, offset + 1);
          ++offset;
          continue;
        }

        if (character == '\'') {
          state = LexerState::kCode;
        }

        break;
      }

      case LexerState::kRawStringLiteral: {
        if (character != '\n') {
          MaskCharacter(result.sanitized_source, offset);
        }

        if (character != ')') {
          break;
        }

        const std::string closing_sequence = ")" + raw_delimiter + "\"";

        if (source.substr(offset, closing_sequence.size()) !=
            closing_sequence) {
          break;
        }

        for (std::size_t current = offset;
             current < offset + closing_sequence.size(); ++current) {
          MaskCharacter(result.sanitized_source, current);
        }

        offset += closing_sequence.size() - 1;
        raw_delimiter.clear();
        state = LexerState::kCode;
        break;
      }

      case LexerState::kPreprocessorDirective: {
        if (character == '\n') {
          if (!IsDirectiveContinued(source, offset)) {
            state = LexerState::kCode;
            at_line_start = true;
          }

          continue;
        }

        MaskCharacter(result.sanitized_source, offset);
        break;
      }
    }
  }

  switch (state) {
    case LexerState::kBlockComment:
      return std::unexpected(UnterminatedBlockComment(
          file_path, LineFromOffset(result.line_starts, construct_start),
          construct_start));

    case LexerState::kStringLiteral:
      return std::unexpected(UnterminatedStringLiteral(
          file_path, LineFromOffset(result.line_starts, construct_start),
          construct_start));

    case LexerState::kCharacterLiteral:
      return std::unexpected(UnterminatedCharacterLiteral(
          file_path, LineFromOffset(result.line_starts, construct_start),
          construct_start));

    case LexerState::kRawStringLiteral:
      return std::unexpected(UnterminatedRawStringLiteral(
          file_path, LineFromOffset(result.line_starts, construct_start),
          construct_start));

    case LexerState::kCode:
    case LexerState::kLineComment:
    case LexerState::kPreprocessorDirective:
      break;
  }

  return result;
}

std::expected<StructureInfo, ParseError> BuildStructure(
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

std::size_t FindPreviousBoundary(std::string_view source,
                                 std::size_t offset) {
  while (offset > 0) {
    --offset;

    switch (source[offset]) {
      case ';':
      case '{':
      case '}':
        return offset + 1;

      default:
        break;
    }
  }

  return 0;
}

std::size_t SkipWhitespaceForward(std::string_view source,
                                  std::size_t offset,
                                  std::size_t end_offset) {
  while (offset < end_offset && IsSpace(source[offset])) {
    ++offset;
  }

  return offset;
}

bool IsTokenBoundary(std::string_view source, std::size_t offset) {
  return offset >= source.size() || !IsIdentifierCharacter(source[offset]);
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

CodeEntityInfo MakeEntityInfo(
    const EntityCandidate& candidate,
    const std::vector<std::size_t>& line_starts,
    const std::filesystem::path& file_path) {
  return CodeEntityInfo{
      .type = candidate.type,
      .name = candidate.name,
      .file_path = file_path,
      .start_line = LineFromOffset(line_starts, candidate.start_offset),
      .end_line = LineFromOffset(line_starts, candidate.closing_brace),
      .start_offset = candidate.start_offset,
      .end_offset = candidate.end_offset,
      .body_start_offset = candidate.opening_brace,
      .body_end_offset = candidate.closing_brace + 1,
  };
}

bool HasExplicitFunctionQualifier(std::string_view name) {
  const std::size_t operator_position = name.find("operator");

  if (operator_position != std::string_view::npos) {
    return name.substr(0, operator_position).find("::") !=
           std::string_view::npos;
  }

  return name.find("::") != std::string_view::npos;
}

const EntityCandidate* FindContainingType(
    const std::vector<EntityCandidate>& type_candidates,
    const EntityCandidate& function_candidate) {
  const EntityCandidate* result = nullptr;
  std::size_t best_size = kNoOffset;

  for (const EntityCandidate& type_candidate : type_candidates) {
    if (function_candidate.opening_brace <= type_candidate.opening_brace ||
        function_candidate.opening_brace >= type_candidate.closing_brace) {
      continue;
    }

    const std::size_t type_size =
        type_candidate.closing_brace - type_candidate.opening_brace;

    if (type_size < best_size) {
      best_size = type_size;
      result = &type_candidate;
    }
  }

  return result;
}

void QualifyMemberFunction(
    EntityCandidate& function_candidate,
    const std::vector<EntityCandidate>& type_candidates) {
  if (function_candidate.is_friend ||
      HasExplicitFunctionQualifier(function_candidate.name)) {
    return;
  }

  const EntityCandidate* containing_type =
      FindContainingType(type_candidates, function_candidate);

  if (containing_type == nullptr) {
    return;
  }

  function_candidate.name =
      containing_type->name + "::" + function_candidate.name;
}

std::vector<EntityCandidate> FindTypeCandidates(
    std::string_view source, const StructureInfo& structure) {
  std::vector<EntityCandidate> candidates;

  for (std::size_t offset = 0; offset < source.size(); ++offset) {
    if (source[offset] != '{') {
      continue;
    }

    if (auto candidate = TryFindTypeCandidate(source, structure, offset)) {
      candidates.push_back(std::move(*candidate));
    }
  }

  return candidates;
}

const EntityCandidate* FindTypeCandidateAt(
    const std::vector<EntityCandidate>& type_candidates,
    std::size_t opening_brace) {
  for (const EntityCandidate& candidate : type_candidates) {
    if (candidate.opening_brace == opening_brace) {
      return &candidate;
    }
  }

  return nullptr;
}

CodeEntities FindEntities(
    std::string_view source,
    const StructureInfo& structure,
    const std::vector<std::size_t>& line_starts,
    const std::filesystem::path& file_path) {
  CodeEntities entities;
  const std::vector<EntityCandidate> type_candidates =
      FindTypeCandidates(source, structure);

  for (std::size_t offset = 0; offset < source.size(); ++offset) {
    if (source[offset] != '{') {
      continue;
    }

    if (const EntityCandidate* type_candidate =
            FindTypeCandidateAt(type_candidates, offset);
        type_candidate != nullptr) {
      entities.push_back(
          MakeEntityInfo(*type_candidate, line_starts, file_path));
      continue;
    }

    auto function_candidate =
        TryFindFunctionCandidate(source, structure, offset);

    if (!function_candidate) {
      continue;
    }

    QualifyMemberFunction(*function_candidate, type_candidates);
    entities.push_back(
        MakeEntityInfo(*function_candidate, line_starts, file_path));

    offset = function_candidate->closing_brace;
  }

  return entities;
}

}  // namespace

std::expected<CodeEntities, ParseError> SimpleSourceParser::Parse(
    std::string_view source, const std::filesystem::path& file_path) const {
  auto lex_result = Lex(source, file_path);

  if (!lex_result) {
    return std::unexpected(std::move(lex_result.error()));
  }

  auto structure = BuildStructure(lex_result->sanitized_source,
                                  lex_result->line_starts,
                                  file_path);

  if (!structure) {
    return std::unexpected(std::move(structure.error()));
  }

  return FindEntities(lex_result->sanitized_source,
                      *structure,
                      lex_result->line_starts,
                      file_path);
}

}  // namespace cpp_defense
