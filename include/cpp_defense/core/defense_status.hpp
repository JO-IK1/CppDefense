#pragma once

namespace cpp_defense {

enum class DefenseStatus {
  kIdle,
  kPreparing,
  kWorkspaceReady,
  kActive,
  kChecking,
  kSuccess,
  kFailed,
  kExpired,
  kError,
};

}  // namespace cpp_defense
