#include "cpp_defense/core/candidate_picker.hpp"

#include <algorithm>
#include <cstddef>
#include <cstdint>
#include <expected>
#include <random>
#include <utility>
#include <vector>

#include "cpp_defense/core/code_entity_info.hpp"
#include "cpp_defense/core/picker_error.hpp"

namespace cpp_defense {
namespace {

bool IsAllowedCandidate(const CodeEntityInfo& entity,
                        CandidateSelectionMode mode) {
  return mode == CandidateSelectionMode::kAll ||
         entity.type == CodeEntityType::kFunction;
}

}  // namespace

CandidatePicker::CandidatePicker()
    : generator_(std::random_device{}()) {}

CandidatePicker::CandidatePicker(std::uint64_t seed)
    : generator_(seed) {}

std::expected<CandidateSelection, PickerError> CandidatePicker::Pick(
    const std::vector<CodeEntityInfo>& entities,
    std::size_t candidate_count,
    CandidateSelectionMode mode) {
  if (candidate_count == 0) {
    return std::unexpected(InvalidCandidateCount());
  }

  std::vector<CodeEntityInfo> eligible;
  for (const CodeEntityInfo& entity : entities) {
    if (IsAllowedCandidate(entity, mode)) eligible.push_back(entity);
  }
  if (eligible.empty()) {
    return std::unexpected(NoSuitableCandidates());
  }

  std::sort(eligible.begin(), eligible.end(), CandidatePriorityCompare{});
  const auto actual_count = std::min(candidate_count, eligible.size());
  const auto random_count = actual_count == 1 ? std::size_t{0}
                                               : (actual_count + 1) / 3;
  const auto largest_count = actual_count - random_count;

  CandidateQueue candidates(actual_count);
  for (std::size_t i = 0; i < largest_count; ++i) {
    candidates.push(eligible[i]);
  }
  std::shuffle(eligible.begin() + static_cast<std::ptrdiff_t>(largest_count),
               eligible.end(), generator_);
  for (std::size_t i = largest_count;
       i < largest_count + random_count; ++i) {
    candidates.push(eligible[i]);
  }

  const std::uint64_t bound = candidates.size();
  const std::uint64_t threshold = (std::uint64_t{0} - bound) % bound;
  std::uint64_t value;
  do { value = generator_(); } while (value < threshold);
  const std::size_t selected_index = static_cast<std::size_t>(value % bound);

  return CandidateSelection{
      .candidates = std::move(candidates),
      .selected_index = selected_index,
  };
}

}  // namespace cpp_defense
