#pragma once

#include <expected>
#include <filesystem>
#include <string>

#include "cpp_defense/core/source_file_error.hpp"

namespace cpp_defense {

class SourceFileRepository {
 public:
  std::expected<std::string, SourceFileError> ReadFile(
      const std::filesystem::path& file_path) const;
};

}  // namespace cpp_defense
