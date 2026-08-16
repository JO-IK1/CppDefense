#pragma once

#include <string>
#include <utility>

namespace cpp_defense {

enum class PickerErrorType {
  kNoSuitableCandidates,
  kInvalidCandidateCount,
};

struct PickerError {
  PickerErrorType type;
  std::string message;

  PickerError(PickerErrorType error_type, std::string error_message)
      : type(error_type), message(std::move(error_message)) {}

  std::string FullMessage() const {
    return message;
  }
};

inline PickerError NoSuitableCandidates() {
  return PickerError(PickerErrorType::kNoSuitableCandidates,
                     "No suitable defense candidates found");
}

inline PickerError InvalidCandidateCount() {
  return PickerError(PickerErrorType::kInvalidCandidateCount,
                     "Candidate count must be greater than zero");
}

}  // namespace cpp_defense
