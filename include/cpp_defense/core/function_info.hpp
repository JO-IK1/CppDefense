#pragma once

#include <cstddef>
#include <filesystem>
#include <string>

namespace cpp_defense {

struct FunctionInfo {
  std::string name;
  std::filesystem::path file_path;
  std::size_t start_line = 0;
  std::size_t end_line = 0;
  std::size_t start_offset = 0;
  std::size_t end_offset = 0;
  std::size_t size = 0;
};

}  // namespace cpp_defense
