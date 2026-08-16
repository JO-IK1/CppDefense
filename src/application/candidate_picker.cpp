#include "cpp_defense/application/candidate_picker.hpp"

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

CandidatePicker::CandidatePicker(std::uint32_t seed)
    : generator_(seed) {}

std::expected<CandidateSelection, PickerError> CandidatePicker::Pick(
    const std::vector<CodeEntityInfo>& entities,
    std::size_t candidate_count,
    CandidateSelectionMode mode) {
  if (candidate_count == 0) {
    return std::unexpected(InvalidCandidateCount());
  }

  CandidateQueue candidates(candidate_count);

  for (const CodeEntityInfo& entity : entities) {
    if (!IsAllowedCandidate(entity, mode)) {
      continue;
    }

    candidates.push(entity);
  }

  if (candidates.empty()) {
    return std::unexpected(NoSuitableCandidates());
  }

  std::uniform_int_distribution<std::size_t> distribution(
      0, candidates.size() - 1);
  const std::size_t selected_index = distribution(generator_);

  return CandidateSelection{
      .candidates = std::move(candidates),
      .selected_index = selected_index,
  };
}

}  // namespace cpp_defense
