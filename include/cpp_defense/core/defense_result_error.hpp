#pragma once

#include <filesystem>
#include <string>
#include <utility>

namespace cpp_defense {

struct DefenseResultError {
  std::string message;
  std::filesystem::path problematic_path;

  DefenseResultError(std::string error_message, std::filesystem::path path)
      : message(std::move(error_message)), problematic_path(std::move(path)) {}

  std::string FullMessage() const {
    return message + ". Path: " + problematic_path.string();
  }
};

}  // namespace cpp_defense
