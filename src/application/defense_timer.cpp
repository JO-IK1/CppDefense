#include "cpp_defense/application/defense_timer.hpp"

#include <algorithm>
#include <utility>

namespace cpp_defense {

DefenseTimer::DefenseTimer()
    : DefenseTimer([] { return Clock::now(); }) {}

DefenseTimer::DefenseTimer(NowFunction now_function)
    : now_function_(std::move(now_function)) {}

void DefenseTimer::Start(std::chrono::seconds duration) {
  deadline_ = now_function_() + duration;
  running_ = true;
}

void DefenseTimer::Stop() noexcept {
  running_ = false;
}

bool DefenseTimer::running() const noexcept {
  return running_;
}

bool DefenseTimer::expired() const {
  return running_ && now_function_() >= deadline_;
}

std::chrono::seconds DefenseTimer::remaining() const {
  if (!running_) {
    return std::chrono::seconds::zero();
  }

  const auto remaining_duration = deadline_ - now_function_();
  if (remaining_duration <= Clock::duration::zero()) {
    return std::chrono::seconds::zero();
  }

  return std::chrono::duration_cast<std::chrono::seconds>(remaining_duration);
}

}  // namespace cpp_defense
