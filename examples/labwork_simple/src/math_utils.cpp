#include "math_utils.hpp"

#include <algorithm>
#include <numeric>

namespace {

int Clamp(int value, int low, int high) {
  return std::max(low, std::min(value, high));
}

}  // namespace

double Mean(const std::vector<int>& values) {
  if (values.empty()) {
    return 0.0;
  }
  const int sum = std::accumulate(values.begin(), values.end(), 0);
  return static_cast<double>(sum) / static_cast<double>(values.size());
}

bool ValidateData(const std::vector<int>& values) {
  if (values.empty()) {
    return false;
  }
  for (int value : values) {
    if (value < -1000 || value > 1000) {
      return false;
    }
  }
  return true;
}

int CalculateScore(const std::vector<int>& values) {
  int score = 0;
  for (int value : values) {
    if (value > 0) {
      score += value * 2;
    } else if (value < 0) {
      score += value;
    }
  }
  return score;
}

std::vector<int> Normalize(const std::vector<int>& values) {
  std::vector<int> result;
  result.reserve(values.size());
  for (int value : values) {
    result.push_back(Clamp(value, 0, 100));
  }
  return result;
}

double CalculateStatistics(const std::vector<int>& values) {
  if (!ValidateData(values)) {
    return 0.0;
  }

  const std::vector<int> normalized = Normalize(values);
  const double mean = Mean(normalized);
  const int score = CalculateScore(normalized);

  int minimum = normalized.front();
  int maximum = normalized.front();
  for (int value : normalized) {
    minimum = std::min(minimum, value);
    maximum = std::max(maximum, value);
  }

  const double spread = static_cast<double>(maximum - minimum);
  const double score_component = static_cast<double>(score) / 100.0;
  return mean + spread + score_component;
}
