#pragma once

#include <algorithm>
#include <cctype>
#include <cstddef>
#include <iterator>
#include <limits>
#include <string>
#include <string_view>
#include <vector>

#include "cpp_defense/core/code_entity_info.hpp"

namespace cpp_defense::source_parser_internal {

inline constexpr std::size_t kNoOffset =
    std::numeric_limits<std::size_t>::max();

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

inline bool IsSpace(char character) {
  return std::isspace(static_cast<unsigned char>(character)) != 0;
}

inline bool IsIdentifierStart(char character) {
  const unsigned char value = static_cast<unsigned char>(character);
  return std::isalpha(value) != 0 || character == '_';
}

inline bool IsIdentifierCharacter(char character) {
  const unsigned char value = static_cast<unsigned char>(character);
  return std::isalnum(value) != 0 || character == '_';
}

inline std::size_t LineFromOffset(
    const std::vector<std::size_t>& line_starts,
    std::size_t offset) {
  const auto iterator =
      std::upper_bound(line_starts.begin(), line_starts.end(), offset);

  return static_cast<std::size_t>(
      std::distance(line_starts.begin(), iterator));
}

inline std::size_t FindPreviousBoundary(std::string_view source,
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

inline std::size_t SkipWhitespaceForward(std::string_view source,
                                         std::size_t offset,
                                         std::size_t end_offset) {
  while (offset < end_offset && IsSpace(source[offset])) {
    ++offset;
  }

  return offset;
}

}  // namespace cpp_defense::source_parser_internal
