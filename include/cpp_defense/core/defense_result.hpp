#pragma once

#include <chrono>
#include <cstddef>
#include <string>

#include "cpp_defense/core/code_entity_info.hpp"
#include "cpp_defense/core/defense_status.hpp"

namespace cpp_defense {

struct DefenseResult {
  CodeEntityInfo selected_entity;
  DefenseStatus status = DefenseStatus::kFailed;
  std::size_t attempts = 0;
  std::chrono::seconds elapsed_time{0};
  std::string last_build_log;

  bool success() const noexcept {
    return status == DefenseStatus::kSuccess;
  }
};

}  // namespace cpp_defense
