#include <iostream>
#include <vector>

#include "math_utils.hpp"

int main() {
  const std::vector<int> values{10, 20, 30, 40, 50};
  std::cout << CalculateStatistics(values) << '\n';
  return 0;
}
