#pragma once

#include <chrono>
#include <functional>

namespace cpp_defense {

class DefenseTimer {
 public:
  using Clock = std::chrono::steady_clock;
  using TimePoint = Clock::time_point;
  using NowFunction = std::function<TimePoint()>;

  DefenseTimer();
  explicit DefenseTimer(NowFunction now_function);

  void Start(std::chrono::seconds duration);
  void Stop() noexcept;

  bool running() const noexcept;
  bool expired() const;
  std::chrono::seconds remaining() const;

 private:
  NowFunction now_function_;
  TimePoint deadline_{};
  bool running_ = false;
};

}  // namespace cpp_defense
