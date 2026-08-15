#pragma once

#include <string>
#include <utility>
#include <variant>

#include "cpp_defense/core/cache_error.hpp"
#include "cpp_defense/core/scan_error.hpp"

namespace cpp_defense {

struct ProjectPreparationError {
  explicit ProjectPreparationError(CacheError error)
      : cause(std::in_place_type<CacheError>, std::move(error)) {}

  explicit ProjectPreparationError(ScanError error)
      : cause(std::in_place_type<ScanError>, std::move(error)) {}

  [[nodiscard]] std::string FullMessage() const {
    return std::visit(
        [](const auto& error) {
          return error.FullMessage();
        },
        cause);
  }

  std::variant<CacheError, ScanError> cause;
};

}  // namespace cpp_defense
