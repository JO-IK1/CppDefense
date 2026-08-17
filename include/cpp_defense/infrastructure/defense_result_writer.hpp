#pragma once

#include <expected>
#include <filesystem>

#include "cpp_defense/core/defense_result.hpp"
#include "cpp_defense/core/defense_result_error.hpp"

namespace cpp_defense {

class DefenseResultWriter {
 public:
  std::expected<void, DefenseResultError> Write(
      const DefenseResult& result,
      const std::filesystem::path& output_path) const;
};

}  // namespace cpp_defense
