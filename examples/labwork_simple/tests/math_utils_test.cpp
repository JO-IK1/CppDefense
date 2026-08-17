#include <cmath>
#include <vector>

#include "math_utils.hpp"

int main() {
  const std::vector<int> values{10, 20, 30};
  if (!ValidateData(values)) {
    return 1;
  }
  if (Mean(values) != 20.0) {
    return 1;
  }
  if (Normalize({-10, 50, 120}) != std::vector<int>({0, 50, 100})) {
    return 1;
  }
  return CalculateStatistics(values) > 0.0 ? 0 : 1;
}
