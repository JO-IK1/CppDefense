#include <array>
#include <chrono>
#include <iostream>
#include <string_view>

#include "cpp_defense/application/defense_timer.hpp"

namespace {
using cpp_defense::DefenseTimer;
using namespace std::chrono_literals;
constexpr int kSuccess = 0;
constexpr int kFailure = 1;

bool Expect(bool condition, std::string_view message) {
  if (!condition) std::cerr << "FAILED: " << message << '\n';
  return condition;
}

bool TestExpiration() {
  DefenseTimer::TimePoint now{};
  DefenseTimer timer([&now] { return now; });
  timer.Start(10s);
  const bool initially = timer.running() && !timer.expired() && timer.remaining() == 10s;
  now += 11s;
  return Expect(initially, "timer starts with full duration") &&
         Expect(timer.expired(), "timer expires after deadline") &&
         Expect(timer.remaining() == 0s, "expired timer has zero remaining");
}

bool TestStop() {
  DefenseTimer::TimePoint now{};
  DefenseTimer timer([&now] { return now; });
  timer.Start(10s);
  timer.Stop();
  now += 20s;
  return Expect(!timer.running(), "stopped timer is not running") &&
         Expect(!timer.expired(), "stopped timer does not expire") &&
         Expect(timer.remaining() == 0s, "stopped timer has zero remaining");
}

struct TestCase { std::string_view name; bool (*fn)(); };
constexpr std::array<TestCase, 2> kTests{{
    {"expiration", TestExpiration},
    {"stop", TestStop},
}};
}

int main(int argc, char* argv[]) {
  if (argc != 2) return kFailure;
  for (const auto& test : kTests) {
    if (test.name == argv[1]) return test.fn() ? kSuccess : kFailure;
  }
  std::cerr << "Unknown test case: " << argv[1] << '\n';
  return kFailure;
}
