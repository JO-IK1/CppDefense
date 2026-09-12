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
                  const CodeEntityInfo& rhs) const {
    if (lhs.body_line_count() != rhs.body_line_count()) {
      return lhs.body_line_count() > rhs.body_line_count();
    }
    if (lhs.file_path != rhs.file_path) {
      return lhs.file_path.generic_string() < rhs.file_path.generic_string();
    }
    return lhs.body_start_offset < rhs.body_start_offset;
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
  explicit CandidatePicker(std::uint64_t seed);

  std::expected<CandidateSelection, PickerError> Pick(
      const std::vector<CodeEntityInfo>& entities,
      std::size_t candidate_count,
      CandidateSelectionMode mode);

 private:
  std::mt19937_64 generator_;
};

}  // namespace cpp_defense
