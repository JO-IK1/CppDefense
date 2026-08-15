#pragma once

#include <cstddef>
#include <filesystem>
#include <string>

namespace cpp_defense {

enum class CodeEntityType {
  kFunction,
  kClass,
  kStruct,
};

struct CodeEntityInfo {
  CodeEntityType type;
  std::string name;
  std::filesystem::path file_path;

  std::size_t start_line = 0;
  std::size_t end_line = 0;

  std::size_t start_offset = 0;
  std::size_t end_offset = 0;

  std::size_t body_start_offset = 0;
  std::size_t body_end_offset = 0;

  [[nodiscard]] std::size_t size() const noexcept {
    return end_offset - start_offset;
  }

  [[nodiscard]] std::size_t body_size() const noexcept {
    return body_end_offset - body_start_offset;
  }
};

}  // namespace cpp_defense
