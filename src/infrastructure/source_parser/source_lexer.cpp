#include "source_lexer.hpp"

#include <algorithm>
#include <cstddef>
#include <expected>
#include <filesystem>
#include <string>
#include <string_view>
#include <vector>

namespace cpp_defense::source_parser_internal {
namespace {

enum class LexerState {
  kCode,
  kLineComment,
  kBlockComment,
  kStringLiteral,
  kCharacterLiteral,
  kRawStringLiteral,
  kPreprocessorDirective,
};

void MaskCharacter(std::string& source, std::size_t offset) {
  if (source[offset] != '\n' && source[offset] != '\r') {
    source[offset] = ' ';
  }
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


}  // namespace

std::expected<LexResult, ParseError> LexSource(
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

}  // namespace cpp_defense::source_parser_internal
