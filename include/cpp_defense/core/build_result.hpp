#pragma once

#include <string>

namespace cpp_defense {

struct BuildStepResult {
  bool attempted = false;
  bool succeeded = false;
  int exit_code = -1;
  std::string output;
};

struct BuildResult {
  BuildStepResult configure;
  BuildStepResult build;
  BuildStepResult tests;

  bool success() const noexcept {
    return configure.succeeded && build.succeeded && tests.succeeded;
  }
};

}  // namespace cpp_defense
