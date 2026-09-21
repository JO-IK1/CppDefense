#include <algorithm>
#include <array>
#include <iostream>
#include <stdexcept>
#include <string_view>
#include <utility>
#include <vector>

#include "cpp_defense/core/fixed_priority_queue.hpp"

namespace {

using cpp_defense::FixedPriorityQueue;

constexpr int kSuccessExitCode = 0;
constexpr int kFailureExitCode = 1;

bool Expect(bool condition, std::string_view message) {
  if (!condition) {
    std::cerr << "FAILED: " << message << '\n';
  }

  return condition;
}

bool TestKeepsLargestValues() {
  FixedPriorityQueue<int> queue(3);

  queue.push(5);
  queue.push(1);
  queue.push(10);
  queue.push(3);
  queue.push(8);

  std::vector<int> values;
  for (std::size_t index = 0; index < queue.size(); ++index) {
    values.push_back(queue[index]);
  }

  std::sort(values.begin(), values.end());

  return Expect(values == std::vector<int>({5, 8, 10}),
                "fixed queue keeps only the largest values") &&
         Expect(queue.top() == 5,
                "top is the smallest retained value");
}

bool TestRejectsWeakerValueWhenFull() {
  FixedPriorityQueue<int> queue(2);

  queue.push(10);
  queue.push(20);
  const bool inserted = queue.push(5);

  return Expect(!inserted,
                "weaker value is rejected when the queue is full") &&
         Expect(queue.size() == 2,
                "rejected value does not change queue size") &&
         Expect(queue.top() == 10,
                "rejected value does not change queue contents");
}

bool TestPopRemovesTop() {
  FixedPriorityQueue<int> queue(3);

  queue.push(5);
  queue.push(8);
  queue.push(10);
  queue.pop();

  return Expect(queue.size() == 2,
                "pop decreases queue size") &&
         Expect(queue.top() == 8,
                "pop removes the smallest retained value");
}

bool TestEmptyOperations() {
  FixedPriorityQueue<int> queue(3);

  bool top_threw = false;
  bool pop_threw = false;

  try {
    static_cast<void>(queue.top());
  } catch (const std::out_of_range&) {
    top_threw = true;
  }

  try {
    queue.pop();
  } catch (const std::out_of_range&) {
    pop_threw = true;
  }

  return Expect(queue.empty(), "new queue is empty") &&
         Expect(top_threw, "top throws for an empty queue") &&
         Expect(pop_threw, "pop throws for an empty queue");
}


bool TestIndexedAccess() {
  FixedPriorityQueue<int> queue(4);

  queue.push(10);
  queue.push(30);
  queue.push(20);
  queue.push(40);

  bool contains_all = true;
  std::array<bool, 4> seen{false, false, false, false};

  for (std::size_t index = 0; index < queue.size(); ++index) {
    const int value = queue[index];

    if (value == 10) {
      seen[0] = true;
    } else if (value == 20) {
      seen[1] = true;
    } else if (value == 30) {
      seen[2] = true;
    } else if (value == 40) {
      seen[3] = true;
    } else {
      contains_all = false;
    }
  }

  for (bool value_seen : seen) {
    contains_all = contains_all && value_seen;
  }

  return Expect(contains_all,
                "operator[] provides indexed access to retained values");
}

bool TestZeroCapacityRejectsValues() {
  FixedPriorityQueue<int> queue(0);

  const bool inserted = queue.push(42);

  return Expect(!inserted,
                "zero-capacity queue rejects inserted values") &&
         Expect(queue.empty(),
                "zero-capacity queue remains empty") &&
         Expect(queue.size() == 0,
                "zero-capacity queue keeps size zero");
}

struct TestCase {
  std::string_view name;
  bool (*function)();
};

constexpr std::array<TestCase, 6> kTestCases{{
    {"keeps-largest-values", TestKeepsLargestValues},
    {"rejects-weaker-value", TestRejectsWeakerValueWhenFull},
    {"pop-removes-top", TestPopRemovesTop},
    {"empty-operations", TestEmptyOperations},
    {"indexed-access", TestIndexedAccess},
    {"zero-capacity", TestZeroCapacityRejectsValues},
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
