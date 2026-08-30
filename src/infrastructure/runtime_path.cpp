#include "cpp_defense/infrastructure/runtime_path.hpp"

#include <cstdlib>
#include <string_view>
#include <system_error>
#include <utility>

namespace cpp_defense {
namespace {

std::optional<std::filesystem::path> EnvironmentPath(const char* name) {
  const char* value = std::getenv(name);
  if (value == nullptr || std::string_view(value).empty()) {
    return std::nullopt;
  }
  return std::filesystem::path(value);
}

RuntimePlatform CurrentPlatform() {
#ifdef _WIN32
  return RuntimePlatform::kWindows;
#elif defined(__APPLE__)
  return RuntimePlatform::kMacOS;
#else
  return RuntimePlatform::kUnix;
#endif
}

}  // namespace

std::filesystem::path ResolveRuntimeRoot(
    const RuntimeEnvironment& environment, RuntimePlatform platform) {
  if (environment.cpp_defense_home &&
      !environment.cpp_defense_home->empty()) {
    return *environment.cpp_defense_home;
  }

  if (platform == RuntimePlatform::kWindows && environment.local_app_data) {
    return *environment.local_app_data / "CppDefense";
  }
  if (platform == RuntimePlatform::kMacOS && environment.user_home) {
    return *environment.user_home / "Library" / "Caches" / "CppDefense";
  }
  if (platform == RuntimePlatform::kUnix && environment.xdg_cache_home) {
    return *environment.xdg_cache_home / "cpp-defense";
  }
  if (environment.user_home) {
    return *environment.user_home / ".cache" / "cpp-defense";
  }
  return environment.temporary_directory / "cpp-defense";
}

std::filesystem::path ResolveRuntimeRoot() {
  std::error_code error_code;
  std::filesystem::path temporary_directory =
      std::filesystem::temp_directory_path(error_code);
  if (error_code) {
    temporary_directory = std::filesystem::current_path(error_code);
  }

  const RuntimeEnvironment environment{
      .cpp_defense_home = EnvironmentPath("CPP_DEFENSE_HOME"),
      .local_app_data = EnvironmentPath("LOCALAPPDATA"),
      .xdg_cache_home = EnvironmentPath("XDG_CACHE_HOME"),
      .user_home = EnvironmentPath("HOME"),
      .temporary_directory = std::move(temporary_directory),
  };
  return ResolveRuntimeRoot(environment, CurrentPlatform());
}

}  // namespace cpp_defense
