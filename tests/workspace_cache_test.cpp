#include "cpp_defense/infrastructure/workspace_cache.hpp"

#include <array>
#include <chrono>
#include <filesystem>
#include <fstream>
#include <iostream>
#include <iterator>
#include <string>
#include <string_view>
#include <system_error>

namespace {

namespace fs = std::filesystem;
using cpp_defense::CacheErrorType;
using cpp_defense::WorkspaceCache;

class TemporaryDirectory {
 public:
  TemporaryDirectory() {
    const auto suffix = std::chrono::steady_clock::now()
                            .time_since_epoch()
                            .count();
    path_ = fs::temp_directory_path() /
            ("cpp-defense-workspace-cache-test-" + std::to_string(suffix));
    fs::create_directories(path_);
  }

  TemporaryDirectory(const TemporaryDirectory&) = delete;
  TemporaryDirectory& operator=(const TemporaryDirectory&) = delete;

  ~TemporaryDirectory() {
    std::error_code error_code;
    fs::remove_all(path_, error_code);
  }

  const fs::path& path() const noexcept { return path_; }

 private:
  fs::path path_;
};

void WriteFile(const fs::path& path, std::string_view contents) {
  fs::create_directories(path.parent_path());
  std::ofstream output(path);
  output << contents;
}

std::string ReadFile(const fs::path& path) {
  std::ifstream input(path);
  return std::string(std::istreambuf_iterator<char>(input),
                     std::istreambuf_iterator<char>());
}

bool Expect(bool condition, std::string_view message) {
  if (!condition) {
    std::cerr << "FAILED: " << message << '\n';
  }
  return condition;
}

bool TestPrepareWorkspace() {
  TemporaryDirectory temporary_directory;
  const fs::path root = temporary_directory.path() / "CppDefense";
  const fs::path source = temporary_directory.path() / "sample-project";
  fs::create_directories(root / "cache/current");
  fs::create_directories(source / "empty-directory");
  WriteFile(root / "cache/current/stale.txt", "stale");
  WriteFile(source / "src/main.cpp", "int main() { return 0; }");
  WriteFile(source / "README.md", "sample");

  WorkspaceCache cache(root);
  const auto result = cache.PrepareWorkspace(source);

  bool passed = Expect(result.has_value(), "workspace preparation succeeds");
  if (!result) {
    std::cerr << result.error().FullMessage() << '\n';
    return false;
  }

  passed &= Expect(!fs::exists(result->session_root_path / "stale.txt"),
                   "old session is removed");
  passed &= Expect(fs::is_directory(result->build_path),
                   "build directory is created");
  passed &= Expect(fs::is_directory(result->metadata_path),
                   "metadata directory is created");
  passed &= Expect(fs::is_directory(result->logs_path),
                   "logs directory is created");
  passed &= Expect(fs::is_directory(result->cached_project_path /
                                    "empty-directory"),
                   "empty source directories are copied");
  passed &= Expect(ReadFile(result->cached_project_path / "src/main.cpp") ==
                       "int main() { return 0; }",
                   "source files are copied");
  passed &= Expect(ReadFile(source / "README.md") == "sample",
                   "source project remains unchanged");
  return passed;
}

bool TestPrepareFreshWorkspace() {
  TemporaryDirectory temporary_directory;
  const fs::path root = temporary_directory.path() / "CppDefense";
  const fs::path source = temporary_directory.path() / "source-project";
  fs::create_directories(root);
  fs::create_directories(source);
  WriteFile(source / "main.cpp", "int main() { return 0; }");

  WorkspaceCache cache(root);
  const auto result = cache.PrepareWorkspace(source);

  return Expect(result && fs::exists(result->cached_project_path / "main.cpp"),
                "fresh workspace is created when cache does not exist");
}

bool TestRejectsSourceInsideCache() {
  TemporaryDirectory temporary_directory;
  const fs::path root = temporary_directory.path() / "CppDefense";
  const fs::path source = root / "cache/source-project";
  fs::create_directories(source);

  WorkspaceCache cache(root);
  const auto result = cache.PrepareWorkspace(source);

  return Expect(!result &&
                    result.error().type ==
                        CacheErrorType::kSourceProjectInsideCache,
                "source project inside cache is rejected");
}

bool TestRejectsCacheInsideSource() {
  TemporaryDirectory temporary_directory;
  const fs::path source = temporary_directory.path() / "source-project";
  const fs::path root = source / "CppDefense";
  fs::create_directories(root);

  WorkspaceCache cache(root);
  const auto result = cache.PrepareWorkspace(source);

  return Expect(!result &&
                    result.error().type ==
                        CacheErrorType::kCacheInsideSourceProject,
                "cache inside source project is rejected");
}

bool TestRejectsSourceSymlinkBeforeCleanup() {
  TemporaryDirectory temporary_directory;
  const fs::path root = temporary_directory.path() / "CppDefense";
  const fs::path source = temporary_directory.path() / "source-project";
  const fs::path target = temporary_directory.path() / "outside.txt";
  fs::create_directories(root / "cache/current");
  fs::create_directories(source);
  WriteFile(root / "cache/current/stale.txt", "must remain");
  WriteFile(target, "outside");

  std::error_code error_code;
  fs::create_symlink(target, source / "link.txt", error_code);
  if (error_code) {
    std::cout << "SKIPPED: source symlink test: " << error_code.message()
              << '\n';
    return true;
  }

  WorkspaceCache cache(root);
  const auto result = cache.PrepareWorkspace(source);

  bool passed = Expect(
      !result && result.error().type == CacheErrorType::kSymlinkDetected,
      "symlink in source project is rejected");
  passed &= Expect(fs::exists(root / "cache/current/stale.txt"),
                   "validation happens before old session cleanup");
  return passed;
}

bool TestRejectsSymlinkCleanupTarget() {
  TemporaryDirectory temporary_directory;
  const fs::path root = temporary_directory.path() / "CppDefense";
  const fs::path source = temporary_directory.path() / "source-project";
  const fs::path outside = temporary_directory.path() / "outside";
  fs::create_directories(root / "cache");
  fs::create_directories(source);
  fs::create_directories(outside);
  WriteFile(outside / "marker.txt", "must remain");

  std::error_code error_code;
  fs::create_directory_symlink(outside, root / "cache/current", error_code);
  if (error_code) {
    std::cout << "SKIPPED: cleanup symlink test: " << error_code.message()
              << '\n';
    return true;
  }

  WorkspaceCache cache(root);
  const auto result = cache.PrepareWorkspace(source);

  bool passed = Expect(
      !result && result.error().type == CacheErrorType::kSymlinkDetected,
      "symlink cleanup target is rejected");
  passed &= Expect(fs::exists(outside / "marker.txt"),
                   "symlink target remains untouched");
  return passed;
}

struct TestCase {
  std::string_view name;
  bool (*run)();
};

constexpr std::array<TestCase, 6> kTestCases = {{
    {"prepare-existing", TestPrepareWorkspace},
    {"prepare-fresh", TestPrepareFreshWorkspace},
    {"reject-source-inside-cache", TestRejectsSourceInsideCache},
    {"reject-cache-inside-source", TestRejectsCacheInsideSource},
    {"reject-source-symlink", TestRejectsSourceSymlinkBeforeCleanup},
    {"reject-cleanup-symlink", TestRejectsSymlinkCleanupTarget},
}};

}  // namespace

int main(int argc, char* argv[]) {
  if (argc != 2) {
    std::cerr << "Expected exactly one test case name.\n";
    return 2;
  }

  const std::string_view requested_test = argv[1];
  for (const TestCase& test_case : kTestCases) {
    if (test_case.name == requested_test) {
      return test_case.run() ? 0 : 1;
    }
  }

  std::cerr << "Unknown test case: " << requested_test << '\n';
  return 2;
}
