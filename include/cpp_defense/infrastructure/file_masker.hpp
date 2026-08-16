#pragma once

#include <expected>

#include "cpp_defense/core/code_entity_info.hpp"
#include "cpp_defense/core/masker_error.hpp"

namespace cpp_defense {

class FileMasker {
 public:
  std::expected<void, FileMaskerError> Mask(
      const CodeEntityInfo& entity) const;
};

}  // namespace cpp_defense
