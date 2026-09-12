#pragma once

#include <array>
#include <random>
#include <string>
#include <string_view>

namespace cpp_defense {

inline bool IsSessionId(std::string_view id) {
  if (id.size() != 36) return false;
  for (std::size_t i = 0; i < id.size(); ++i) {
    if (i == 8 || i == 13 || i == 18 || i == 23) {
      if (id[i] != '-') return false;
    } else if (!((id[i] >= '0' && id[i] <= '9') ||
                 (id[i] >= 'a' && id[i] <= 'f'))) {
      return false;
    }
  }
  return true;
}

inline std::string NewSessionId() {
  std::random_device random;
  constexpr char hex[] = "0123456789abcdef";
  std::string id(36, '-');
  for (std::size_t i = 0; i < id.size(); ++i) {
    if (i != 8 && i != 13 && i != 18 && i != 23) id[i] = hex[random() & 15];
  }
  id[14] = '4';
  id[19] = hex[8 + (random() & 3)];
  return id;
}
}  // namespace cpp_defense
