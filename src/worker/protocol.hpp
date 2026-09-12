#pragma once
#include <cstdint>
#include <stdexcept>
#include <string>
#include <string_view>
#include <nlohmann/json.hpp>

namespace cpp_defense::worker {
using Json = nlohmann::json;

struct Error : std::runtime_error {
  std::string code;
  std::string category;
  bool retryable;
  Error(std::string code, std::string message,
        std::string category = "invalid_request", bool retryable = false)
      : std::runtime_error(std::move(message)), code(std::move(code)),
        category(std::move(category)), retryable(retryable) {}
};
void Require(bool condition, std::string_view code, std::string_view message);
Json Parse(std::string_view text);
void ValidateRequest(const Json& request);
std::uint64_t Seed(const Json& value);
bool RelativePath(std::string_view value);
Json Failure(const Json& envelope, const Error& error);
}  // namespace cpp_defense::worker
