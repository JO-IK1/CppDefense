#pragma once

#include <filesystem>
#include <optional>

namespace cpp_defense {

enum class RuntimePlatform { kWindows, kMacOS, kUnix };

struct RuntimeEnvironment {
  std::optional<std::filesystem::path> cpp_defense_home;
  std::optional<std::filesystem::path> local_app_data;
  std::optional<std::filesystem::path> xdg_cache_home;
  std::optional<std::filesystem::path> user_home;
  std::filesystem::path temporary_directory;
};

std::filesystem::path ResolveRuntimeRoot(
    const RuntimeEnvironment& environment, RuntimePlatform platform);
std::filesystem::path ResolveRuntimeRoot();

}  // namespace cpp_defense
