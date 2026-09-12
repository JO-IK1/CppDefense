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

  CandidateQueue candidates(candidate_count);

  auto ordered = entities;
  std::sort(ordered.begin(), ordered.end(), CandidatePriorityCompare{});
  for (const CodeEntityInfo& entity : ordered) {
    if (!IsAllowedCandidate(entity, mode)) {
      continue;
    }

    candidates.push(entity);
  }

  if (candidates.empty()) {
    return std::unexpected(NoSuitableCandidates());
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
