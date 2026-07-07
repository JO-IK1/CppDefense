#pragma once

#include <filesystem>
#include <utility>

namespace cpp_defense {

class CliOptions {
 public:
  static constexpr int kDefaultFunctionCount = 5;
  static constexpr int kDefaultTimerMinutes = 5;

  CliOptions() = default;

  explicit CliOptions(std::filesystem::path project_path)
      : project_path_(std::move(project_path)) {}

  const std::filesystem::path& project_path() const noexcept {
    return project_path_;
  }

  int function_count() const noexcept { return function_count_; }

  int timer_minutes() const noexcept { return timer_minutes_; }

  bool functions_only() const noexcept { return functions_only_; }

  void set_project_path(std::filesystem::path project_path) {
    project_path_ = std::move(project_path);
  }

  void set_function_count(int function_count) noexcept {
    function_count_ = function_count;
  }

  void set_timer_minutes(int timer_minutes) noexcept {
    timer_minutes_ = timer_minutes;
  }

  void set_functions_only(bool functions_only) noexcept {
    functions_only_ = functions_only;
  }

 private:
  std::filesystem::path project_path_;
  int function_count_ = kDefaultFunctionCount;
  int timer_minutes_ = kDefaultTimerMinutes;
  bool functions_only_ = true;
};

}  // namespace cpp_defense
