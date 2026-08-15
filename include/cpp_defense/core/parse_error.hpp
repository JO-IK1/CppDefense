#pragma once

#include <cstddef>
#include <filesystem>
#include <string>
#include <utility>

namespace cpp_defense {

enum class ParseErrorType {
  kUnterminatedBlockComment,
  kUnterminatedStringLiteral,
  kUnterminatedCharacterLiteral,
  kUnterminatedRawStringLiteral,
  kUnmatchedOpeningParenthesis,
  kUnmatchedClosingParenthesis,
  kUnmatchedOpeningBrace,
  kUnmatchedClosingBrace,
};

struct ParseError {
  ParseErrorType type;
  std::string message;
  std::filesystem::path problematic_path;
  std::size_t line = 1;
  std::size_t offset = 0;

  ParseError(ParseErrorType error_type, std::string error_message,
             std::filesystem::path path, std::size_t error_line,
             std::size_t error_offset)
      : type(error_type), message(std::move(error_message)),
        problematic_path(std::move(path)), line(error_line),
        offset(error_offset) {}

  std::string FullMessage() const {
    std::string full = message;

    if (!problematic_path.empty()) {
      full += ". Path: " + problematic_path.string();
    }

    full += ". Line: " + std::to_string(line);
    full += ". Offset: " + std::to_string(offset);

    return full;
  }
};

inline ParseError UnterminatedBlockComment(const std::filesystem::path& path,
                                           std::size_t line, std::size_t offset) {
  return ParseError(ParseErrorType::kUnterminatedBlockComment,
                    "Unterminated block comment", path, line, offset);
}

inline ParseError UnterminatedStringLiteral(const std::filesystem::path& path,
                                            std::size_t line, std::size_t offset) {
  return ParseError(ParseErrorType::kUnterminatedStringLiteral,
                    "Unterminated string literal", path, line, offset);
}

inline ParseError UnterminatedCharacterLiteral(const std::filesystem::path& path,
                                               std::size_t line, std::size_t offset) {
  return ParseError(ParseErrorType::kUnterminatedCharacterLiteral,
                    "Unterminated character literal", path, line, offset);
}

inline ParseError UnterminatedRawStringLiteral(const std::filesystem::path& path,
                                               std::size_t line, std::size_t offset) {
  return ParseError(ParseErrorType::kUnterminatedRawStringLiteral,
                    "Unterminated raw string literal", path, line, offset);
}

inline ParseError UnmatchedOpeningParenthesis(const std::filesystem::path& path,
                                              std::size_t line, std::size_t offset) {
  return ParseError(ParseErrorType::kUnmatchedOpeningParenthesis,
                    "Unmatched opening parenthesis", path, line, offset);
}

inline ParseError UnmatchedClosingParenthesis(const std::filesystem::path& path,
                                              std::size_t line, std::size_t offset) {
  return ParseError(ParseErrorType::kUnmatchedClosingParenthesis,
                    "Unmatched closing parenthesis", path, line, offset);
}

inline ParseError UnmatchedOpeningBrace(const std::filesystem::path& path,
                                        std::size_t line, std::size_t offset) {
  return ParseError(ParseErrorType::kUnmatchedOpeningBrace,
                    "Unmatched opening brace", path, line, offset);
}

inline ParseError UnmatchedClosingBrace(const std::filesystem::path& path,
                                        std::size_t line, std::size_t offset) {
  return ParseError(ParseErrorType::kUnmatchedClosingBrace,
                    "Unmatched closing brace", path, line, offset);
}

}  // namespace cpp_defense
