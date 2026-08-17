#pragma once

#include <vector>

double Mean(const std::vector<int>& values);
bool ValidateData(const std::vector<int>& values);
int CalculateScore(const std::vector<int>& values);
std::vector<int> Normalize(const std::vector<int>& values);
double CalculateStatistics(const std::vector<int>& values);
