#pragma once

#include <cstddef>
#include <cstdint>
#include <expected>
#include <random>
#include <vector>

#include "cpp_defense/core/code_entity_info.hpp"
#include "cpp_defense/core/fixed_priority_queue.hpp"
#include "cpp_defense/core/picker_error.hpp"

namespace cpp_defense {

enum class CandidateSelectionMode {
  kFunctionsOnly,
  kAll,
};

struct CandidatePriorityCompare {
  bool operator()(const CodeEntityInfo& lhs,
                  const CodeEntityInfo& rhs) const noexcept {
    return lhs.body_line_count() > rhs.body_line_count();
  }
};

using CandidateQueue =
    FixedPriorityQueue<CodeEntityInfo, CandidatePriorityCompare>;

struct CandidateSelection {
  CandidateQueue candidates;
  std::size_t selected_index = 0;

  const CodeEntityInfo& selected() const {
    return candidates[selected_index];
  }
};

class CandidatePicker {
 public:
  CandidatePicker();
  explicit CandidatePicker(std::uint32_t seed);

  std::expected<CandidateSelection, PickerError> Pick(
      const std::vector<CodeEntityInfo>& entities,
      std::size_t candidate_count,
      CandidateSelectionMode mode);

 private:
  std::mt19937 generator_;
};

}  // namespace cpp_defense
