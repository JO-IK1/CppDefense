#pragma once

#include <chrono>
#include <expected>
#include <filesystem>
#include <string>
#include <vector>

#include "cpp_defense/core/build_runner_error.hpp"

namespace cpp_defense::process_runner {

struct Result {
  int exit_code = -1;
  bool timed_out = false;
};

std::expected<Result, BuildRunnerError> Run(
    const std::string& executable,
    const std::vector<std::string>& arguments,
    const std::filesystem::path& log_path,
    std::chrono::milliseconds timeout);

}  // namespace cpp_defense::process_runner
