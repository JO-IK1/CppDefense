#include "source_entity_finder.hpp"

#include <cstddef>
#include <filesystem>
#include <string_view>
#include <utility>
#include <vector>

#include "source_function_parser.hpp"
#include "source_type_parser.hpp"

namespace cpp_defense::source_parser_internal {
namespace {

CodeEntityInfo MakeEntityInfo(
    const EntityCandidate& candidate,
    const std::vector<std::size_t>& line_starts,
    const std::filesystem::path& file_path) {
  return CodeEntityInfo{
      .type = candidate.type,
      .name = candidate.name,
      .file_path = file_path,
      .start_line = LineFromOffset(line_starts, candidate.start_offset),
      .end_line = LineFromOffset(line_starts, candidate.closing_brace),
      .body_start_line = LineFromOffset(line_starts, candidate.opening_brace),
      .body_end_line = LineFromOffset(line_starts, candidate.closing_brace),
      .start_offset = candidate.start_offset,
      .end_offset = candidate.end_offset,
      .body_start_offset = candidate.opening_brace,
      .body_end_offset = candidate.closing_brace + 1,
  };
}

bool HasExplicitFunctionQualifier(std::string_view name) {
  const std::size_t operator_position = name.find("operator");

  if (operator_position != std::string_view::npos) {
    return name.substr(0, operator_position).find("::") !=
           std::string_view::npos;
  }

  return name.find("::") != std::string_view::npos;
}

const EntityCandidate* FindContainingType(
    const std::vector<EntityCandidate>& type_candidates,
    const EntityCandidate& function_candidate) {
  const EntityCandidate* result = nullptr;
  std::size_t best_size = kNoOffset;

  for (const EntityCandidate& type_candidate : type_candidates) {
    if (function_candidate.opening_brace <= type_candidate.opening_brace ||
        function_candidate.opening_brace >= type_candidate.closing_brace) {
      continue;
    }

    const std::size_t type_size = type_candidate.closing_brace - type_candidate.opening_brace;

    if (type_size < best_size) {
      best_size = type_size;
      result = &type_candidate;
    }
  }

  return result;
}

void QualifyMemberFunction(
    EntityCandidate& function_candidate,
    const std::vector<EntityCandidate>& type_candidates) {
  if (function_candidate.is_friend ||
      HasExplicitFunctionQualifier(function_candidate.name)) {
    return;
  }

  const EntityCandidate* containing_type = FindContainingType(type_candidates, function_candidate);

  if (containing_type == nullptr) {
    return;
  }

  function_candidate.name =
      containing_type->name + "::" + function_candidate.name;
}

std::vector<EntityCandidate> FindTypeCandidates(
    std::string_view source, const StructureInfo& structure) {
  std::vector<EntityCandidate> candidates;

  for (std::size_t offset = 0; offset < source.size(); ++offset) {
    if (source[offset] != '{') {
      continue;
    }

    if (auto candidate = TryFindTypeCandidate(source, structure, offset)) {
      candidates.push_back(std::move(*candidate));
    }
  }

  return candidates;
}

const EntityCandidate* FindTypeCandidateAt(
    const std::vector<EntityCandidate>& type_candidates,
    std::size_t opening_brace) {
  for (const EntityCandidate& candidate : type_candidates) {
    if (candidate.opening_brace == opening_brace) {
      return &candidate;
    }
  }

  return nullptr;
}

}  // namespace

std::vector<CodeEntityInfo> FindSourceEntities(
    std::string_view source,
    const StructureInfo& structure,
    const std::vector<std::size_t>& line_starts,
    const std::filesystem::path& file_path) {
  std::vector<CodeEntityInfo> entities;
  const std::vector<EntityCandidate> type_candidates = FindTypeCandidates(source, structure);

  for (std::size_t offset = 0; offset < source.size(); ++offset) {
    if (source[offset] != '{') {
      continue;
    }

    if (const EntityCandidate* type_candidate = FindTypeCandidateAt(type_candidates, offset);
        type_candidate != nullptr) {
      entities.push_back(MakeEntityInfo(*type_candidate, line_starts, file_path));
      continue;
    }

    auto function_candidate = TryFindFunctionCandidate(source, structure, offset);

    if (!function_candidate) {
      continue;
    }

    QualifyMemberFunction(*function_candidate, type_candidates);
    entities.push_back(MakeEntityInfo(*function_candidate, line_starts, file_path));

    offset = function_candidate->closing_brace;
  }

  return entities;
}

}  // namespace cpp_defense::source_parser_internal
