#include <algorithm>
#include <array>
#include <cstddef>
#include <cstdint>
#include <filesystem>
#include <iostream>
#include <string>
#include <string_view>
#include <vector>

#include "cpp_defense/application/candidate_picker.hpp"
#include "cpp_defense/core/code_entity_info.hpp"
#include "cpp_defense/core/picker_error.hpp"

namespace {

using cpp_defense::CandidatePicker;
using cpp_defense::CandidateQueue;
using cpp_defense::CandidateSelectionMode;
using cpp_defense::CodeEntityInfo;
using cpp_defense::CodeEntityType;
using cpp_defense::PickerErrorType;

constexpr int kSuccessExitCode = 0;
constexpr int kFailureExitCode = 1;

bool Expect(bool condition, std::string_view message) {
  if (!condition) {
    std::cerr << "FAILED: " << message << '\n';
  }

  return condition;
}

CodeEntityInfo MakeEntity(std::string name,
                          CodeEntityType type,
                          std::size_t body_line_count) {
  const std::size_t body_start_line = 10;
  const std::size_t body_end_line =
      body_start_line + body_line_count + 1;

  return CodeEntityInfo{
      .type = type,
      .name = std::move(name),
      .file_path = "test.cpp",
      .start_line = body_start_line,
      .end_line = body_end_line,
      .body_start_line = body_start_line,
      .body_end_line = body_end_line,
      .start_offset = 0,
      .end_offset = 100,
      .body_start_offset = 10,
      .body_end_offset = 90,
  };
}

std::vector<std::string> CandidateNames(
    const CandidateQueue& candidates) {
  std::vector<std::string> names;
  names.reserve(candidates.size());

  for (std::size_t index = 0; index < candidates.size(); ++index) {
    names.push_back(candidates[index].name);
  }

  std::sort(names.begin(), names.end());
  return names;
}

bool TestFunctionsOnly() {
  const std::vector<CodeEntityInfo> entities{
      MakeEntity("SmallFunction", CodeEntityType::kFunction, 2),
      MakeEntity("LargeClass", CodeEntityType::kClass, 50),
      MakeEntity("LargeFunction", CodeEntityType::kFunction, 10),
      MakeEntity("Config", CodeEntityType::kStruct, 20),
  };

  CandidatePicker picker(1);
  const auto result = picker.Pick(
      entities, 5, CandidateSelectionMode::kFunctionsOnly);

  if (!Expect(result.has_value(),
              "functions-only selection succeeds")) {
    return false;
  }

  return Expect(
      CandidateNames(result->candidates) ==
          std::vector<std::string>({"LargeFunction", "SmallFunction"}),
      "functions-only mode removes non-functions");
}

bool TestAllEntities() {
  const std::vector<CodeEntityInfo> entities{
      MakeEntity("Function", CodeEntityType::kFunction, 1),
      MakeEntity("Class", CodeEntityType::kClass, 2),
      MakeEntity("Struct", CodeEntityType::kStruct, 3),
      MakeEntity("Enum", CodeEntityType::kEnumClass, 4),
  };

  CandidatePicker picker(2);
  const auto result =
      picker.Pick(entities, 10, CandidateSelectionMode::kAll);

  if (!Expect(result.has_value(), "all-entity selection succeeds")) {
    return false;
  }

  return Expect(result->candidates.size() == 4,
                "all mode keeps every supported entity type");
}

bool TestKeepsLargestCandidates() {
  const std::vector<CodeEntityInfo> entities{
      MakeEntity("One", CodeEntityType::kFunction, 1),
      MakeEntity("Two", CodeEntityType::kFunction, 2),
      MakeEntity("Three", CodeEntityType::kFunction, 3),
      MakeEntity("Four", CodeEntityType::kFunction, 4),
      MakeEntity("Five", CodeEntityType::kFunction, 5),
  };

  CandidatePicker picker(3);
  const auto result = picker.Pick(
      entities, 3, CandidateSelectionMode::kFunctionsOnly);

  if (!Expect(result.has_value(), "top-N selection succeeds")) {
    return false;
  }

  return Expect(
      CandidateNames(result->candidates) ==
          std::vector<std::string>({"Five", "Four", "Three"}),
      "picker keeps the N largest bodies");
}

bool TestUsesAllWhenFewerThanRequested() {
  const std::vector<CodeEntityInfo> entities{
      MakeEntity("First", CodeEntityType::kFunction, 2),
      MakeEntity("Second", CodeEntityType::kFunction, 4),
  };

  CandidatePicker picker(4);
  const auto result = picker.Pick(
      entities, 5, CandidateSelectionMode::kFunctionsOnly);

  return Expect(result.has_value(),
                "selection succeeds with fewer candidates than requested") &&
         Expect(result->candidates.size() == 2,
                "all available candidates are used");
}

bool TestNoSuitableCandidates() {
  const std::vector<CodeEntityInfo> entities{
      MakeEntity("OnlyClass", CodeEntityType::kClass, 10),
  };

  CandidatePicker picker(5);
  const auto result = picker.Pick(
      entities, 5, CandidateSelectionMode::kFunctionsOnly);

  return Expect(!result.has_value(),
                "empty filtered result returns an error") &&
         Expect(result.error().type ==
                    PickerErrorType::kNoSuitableCandidates,
                "empty filtered result reports no candidates");
}

bool TestInvalidCandidateCount() {
  const std::vector<CodeEntityInfo> entities{
      MakeEntity("Function", CodeEntityType::kFunction, 5),
  };

  CandidatePicker picker(6);
  const auto result = picker.Pick(
      entities, 0, CandidateSelectionMode::kFunctionsOnly);

  return Expect(!result.has_value(),
                "zero candidate count returns an error") &&
         Expect(result.error().type ==
                    PickerErrorType::kInvalidCandidateCount,
                "zero candidate count reports invalid count");
}

bool TestFixedSeedIsReproducible() {
  const std::vector<CodeEntityInfo> entities{
      MakeEntity("One", CodeEntityType::kFunction, 1),
      MakeEntity("Two", CodeEntityType::kFunction, 2),
      MakeEntity("Three", CodeEntityType::kFunction, 3),
      MakeEntity("Four", CodeEntityType::kFunction, 4),
  };

  CandidatePicker first_picker(42);
  CandidatePicker second_picker(42);

  const auto first = first_picker.Pick(
      entities, 4, CandidateSelectionMode::kFunctionsOnly);
  const auto second = second_picker.Pick(
      entities, 4, CandidateSelectionMode::kFunctionsOnly);

  if (!Expect(first.has_value() && second.has_value(),
              "fixed-seed selections succeed")) {
    return false;
  }

  return Expect(first->selected_index == second->selected_index,
                "same seed produces the same selected index") &&
         Expect(first->selected().name == second->selected().name,
                "same seed produces the same selected entity");
}

bool TestSelectedEntityComesDirectlyFromQueue() {
  const std::vector<CodeEntityInfo> entities{
      MakeEntity("One", CodeEntityType::kFunction, 1),
      MakeEntity("Two", CodeEntityType::kFunction, 2),
      MakeEntity("Three", CodeEntityType::kFunction, 3),
      MakeEntity("Four", CodeEntityType::kFunction, 4),
      MakeEntity("Five", CodeEntityType::kFunction, 5),
  };

  CandidatePicker picker(17);
  const auto result = picker.Pick(
      entities, 3, CandidateSelectionMode::kFunctionsOnly);

  if (!Expect(result.has_value(), "queue-backed selection succeeds")) {
    return false;
  }

  return Expect(result->selected_index < result->candidates.size(),
                "selected index is inside the candidate queue") &&
         Expect(&result->selected() ==
                    &result->candidates[result->selected_index],
                "selected entity is referenced directly from the queue");
}

struct TestCase {
  std::string_view name;
  bool (*function)();
};

constexpr std::array<TestCase, 8> kTestCases{{
    {"functions-only", TestFunctionsOnly},
    {"all-entities", TestAllEntities},
    {"keeps-largest", TestKeepsLargestCandidates},
    {"fewer-than-requested", TestUsesAllWhenFewerThanRequested},
    {"no-suitable-candidates", TestNoSuitableCandidates},
    {"invalid-candidate-count", TestInvalidCandidateCount},
    {"fixed-seed", TestFixedSeedIsReproducible},
    {"selected-from-queue", TestSelectedEntityComesDirectlyFromQueue},
}};

}  // namespace

int main(int argc, char* argv[]) {
  if (argc != 2) {
    std::cerr << "Expected test case name\n";
    return kFailureExitCode;
  }

  const std::string_view requested_test = argv[1];

  for (const auto& test_case : kTestCases) {
    if (test_case.name == requested_test) {
      return test_case.function()
                 ? kSuccessExitCode
                 : kFailureExitCode;
    }
  }

  std::cerr << "Unknown test case: "
            << requested_test << '\n';

  return kFailureExitCode;
}
