#include <array>
#include <filesystem>
#include <iostream>
#include <string_view>

#include "cpp_defense/infrastructure/runtime_path.hpp"

namespace {
namespace fs = std::filesystem;
using cpp_defense::ResolveRuntimeRoot;
using cpp_defense::RuntimeEnvironment;
using cpp_defense::RuntimePlatform;

bool Expect(bool condition, std::string_view message) {
  if (!condition) std::cerr << "FAILED: " << message << '\n';
  return condition;
}

RuntimeEnvironment Environment() {
  return RuntimeEnvironment{
      .local_app_data = fs::path("C:/Users/test/AppData/Local"),
      .xdg_cache_home = fs::path("/var/cache/user"),
      .user_home = fs::path("/home/test"),
      .temporary_directory = fs::path("/tmp"),
  };
}

bool TestOverride() {
  RuntimeEnvironment environment = Environment();
  environment.cpp_defense_home = fs::path("/custom/runtime");
  return Expect(ResolveRuntimeRoot(environment, RuntimePlatform::kUnix) ==
                    fs::path("/custom/runtime"),
                "explicit runtime root has priority");
}

bool TestPlatforms() {
  const RuntimeEnvironment environment = Environment();
  return Expect(ResolveRuntimeRoot(environment, RuntimePlatform::kWindows) ==
                    fs::path("C:/Users/test/AppData/Local/CppDefense"),
                "Windows uses LOCALAPPDATA") &&
         Expect(ResolveRuntimeRoot(environment, RuntimePlatform::kMacOS) ==
                    fs::path("/home/test/Library/Caches/CppDefense"),
                "macOS uses the user cache directory") &&
         Expect(ResolveRuntimeRoot(environment, RuntimePlatform::kUnix) ==
                    fs::path("/var/cache/user/cpp-defense"),
                "Unix uses XDG_CACHE_HOME");
}

bool TestFallback() {
  const RuntimeEnvironment environment{.temporary_directory = "/tmp"};
  return Expect(ResolveRuntimeRoot(environment, RuntimePlatform::kUnix) ==
                    fs::path("/tmp/cpp-defense"),
                "temporary directory is the final fallback");
}

struct TestCase { std::string_view name; bool (*run)(); };
constexpr std::array<TestCase, 3> kTests{{
    {"override", TestOverride},
    {"platforms", TestPlatforms},
    {"fallback", TestFallback},
}};
}  // namespace

int main(int argc, char* argv[]) {
  if (argc != 2) return 2;
  for (const auto& test : kTests) {
    if (test.name == argv[1]) return test.run() ? 0 : 1;
  }
  return 2;
}
