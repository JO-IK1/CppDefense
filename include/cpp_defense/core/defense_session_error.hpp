#pragma once

#include <string>
#include <utility>

namespace cpp_defense {

enum class DefenseSessionErrorType {
  kNoActiveSession,
  kSessionExpired,
  kPreparationFailed,
  kSourceReadFailed,
  kParseFailed,
  kCandidateSelectionFailed,
  kResultFileFailed,
  kMaskingFailed,
  kCheckWorkspaceFailed,
  kPatchingFailed,
  kBuildFailedToRun,
  kResultWriteFailed,
  kMetadataWriteFailed,
};

struct DefenseSessionError {
  DefenseSessionErrorType type;
  std::string message;

  DefenseSessionError(DefenseSessionErrorType error_type, std::string error_message)
      : type(error_type), message(std::move(error_message)) {}

  const std::string& FullMessage() const noexcept {
    return message;
  }
};

}  // namespace cpp_defense
