#include "cpp_defense/infrastructure/project_scanner.hpp"

#include <algorithm>
#include <array>
#include <chrono>
#include <filesystem>
#include <fstream>
#include <iostream>
#include <string>
#include <string_view>
#include <system_error>

namespace {

namespace fs = std::filesystem;
using cpp_defense::ProjectScanner;
using cpp_defense::ScanErrorType;
using cpp_defense::SourceFilePaths;

class TemporaryDirectory {
 public:
  TemporaryDirectory() {
    const auto suffix =
        std::chrono::steady_clock::now().time_since_epoch().count();
    path_ = fs::temp_directory_path() /
            ("cpp-defense-project-scanner-test-" + std::to_string(suffix));
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

void WriteFile(const fs::path& path) {
  fs::create_directories(path.parent_path());
  std::ofstream output(path);
  output << "test";
}

bool Expect(bool condition, std::string_view message) {
  if (!condition) {
    std::cerr << "FAILED: " << message << '\n';
  }
  return condition;
}

bool ExpectPaths(const SourceFilePaths& actual, SourceFilePaths expected,
                 std::string_view message) {
  std::sort(expected.begin(), expected.end());
  if (actual == expected) {
    return true;
  }

  std::cerr << "FAILED: " << message << "\nExpected:\n";
  for (const fs::path& path : expected) {
    std::cerr << "  " << path << '\n';
  }
  std::cerr << "Actual:\n";
  for (const fs::path& path : actual) {
    std::cerr << "  " << path << '\n';
  }
  return false;
}

bool TestFindsSupportedFiles() {
  TemporaryDirectory temporary_directory;
  const fs::path root = temporary_directory.path() / "project";
  SourceFilePaths expected_paths{
      root / "source.c",            root / "source.cc",
      root / "src/main.cpp",       root / "src/detail/parser.cxx",
      root / "include/types.h",    root / "include/types.hh",
      root / "include/widget.HPP", root / "include/detail.hxx"};
  for (const fs::path& path : expected_paths) {
    WriteFile(path);
  }
  WriteFile(root / "README.md");
  WriteFile(root / "assets/image.png");

  const ProjectScanner scanner;
  const auto result = scanner.FindSourceFiles(root);

  if (!Expect(result.has_value(), "supported source files are found")) {
    std::cerr << result.error().FullMessage() << '\n';
    return false;
  }

  return ExpectPaths(*result, expected_paths,
                     "only supported files are returned in sorted order");
}

bool TestSkipsExcludedDirectories() {
  TemporaryDirectory temporary_directory;
  const fs::path root = temporary_directory.path() / "project";
  constexpr std::array<std::string_view, 7> kExcludedDirectoryNames{
      ".git",  ".idea",             ".vscode", "build",
      "cache", "cmake-build-debug", "cmake-build-release"};

  for (const std::string_view directory_name : kExcludedDirectoryNames) {
    WriteFile(root / directory_name / "ignored.cpp");
  }

  const fs::path included_file = root / "building_model/included.cpp";
  WriteFile(included_file);

  const ProjectScanner scanner;
  const auto result = scanner.FindSourceFiles(root);

  if (!Expect(result.has_value(), "excluded directories are skipped")) {
    std::cerr << result.error().FullMessage() << '\n';
    return false;
  }

  return ExpectPaths(*result, {included_file},
                     "excluded directory contents are not returned");
}

bool TestSkipsSymbolicLinks() {
  TemporaryDirectory temporary_directory;
  const fs::path root = temporary_directory.path() / "project";
  const fs::path outside = temporary_directory.path() / "outside";
  const fs::path included_file = root / "included.cpp";
  const fs::path outside_file = outside / "outside.cpp";
  WriteFile(included_file);
  WriteFile(outside_file);

  std::error_code error_code;
  fs::create_symlink(outside_file, root / "linked-file.cpp", error_code);
  if (error_code) {
    std::cout << "SKIPPED: file symlink test: " << error_code.message()
              << '\n';
    return true;
  }

  fs::create_directory_symlink(outside, root / "linked-directory", error_code);
  if (error_code) {
    std::cout << "SKIPPED: directory symlink test: " << error_code.message()
              << '\n';
    return true;
  }

  const ProjectScanner scanner;
  const auto result = scanner.FindSourceFiles(root);

  if (!Expect(result.has_value(), "symbolic links are skipped")) {
    std::cerr << result.error().FullMessage() << '\n';
    return false;
  }

  return ExpectPaths(*result, {included_file},
                     "symbolic-link targets are not returned");
}

bool TestReturnsEmptyResult() {
  TemporaryDirectory temporary_directory;
  const fs::path root = temporary_directory.path() / "project";
  WriteFile(root / "README.md");

  const ProjectScanner scanner;
  const auto result = scanner.FindSourceFiles(root);

  return Expect(result && result->empty(),
                "project without source files returns an empty result");
}

bool TestRejectsMissingRoot() {
  TemporaryDirectory temporary_directory;
  const fs::path root = temporary_directory.path() / "missing-project";

  const ProjectScanner scanner;
  const auto result = scanner.FindSourceFiles(root);

  return Expect(!result &&
                    result.error().type == ScanErrorType::kScanRootNotFound,
                "missing scan root is rejected");
}

bool TestRejectsFileRoot() {
  TemporaryDirectory temporary_directory;
  const fs::path root = temporary_directory.path() / "project.cpp";
  WriteFile(root);

  const ProjectScanner scanner;
  const auto result = scanner.FindSourceFiles(root);

  return Expect(
      !result &&
          result.error().type == ScanErrorType::kScanRootNotDirectory,
      "regular file cannot be used as the scan root");
}

bool TestRejectsRootSymlink() {
  TemporaryDirectory temporary_directory;
  const fs::path target = temporary_directory.path() / "project";
  const fs::path root = temporary_directory.path() / "project-link";
  fs::create_directories(target);

  std::error_code error_code;
  fs::create_directory_symlink(target, root, error_code);
  if (error_code) {
    std::cout << "SKIPPED: root symlink test: " << error_code.message()
              << '\n';
    return true;
  }

  const ProjectScanner scanner;
  const auto result = scanner.FindSourceFiles(root);

  return Expect(!result &&
                    result.error().type == ScanErrorType::kScanRootIsSymlink,
                "symbolic link cannot be used as the scan root");
}

bool TestRejectsDanglingRootSymlink() {
  TemporaryDirectory temporary_directory;
  const fs::path target = temporary_directory.path() / "missing-project";
  const fs::path root = temporary_directory.path() / "project-link";

  std::error_code error_code;
  fs::create_directory_symlink(target, root, error_code);
  if (error_code) {
    std::cout << "SKIPPED: dangling root symlink test: "
              << error_code.message() << '\n';
    return true;
  }

  const ProjectScanner scanner;
  const auto result = scanner.FindSourceFiles(root);

  return Expect(!result &&
                    result.error().type == ScanErrorType::kScanRootIsSymlink,
                "dangling symbolic link is rejected as the scan root");
}

struct TestCase {
  std::string_view name;
  bool (*run)();
};

constexpr std::array<TestCase, 8> kTestCases = {{
    {"find-supported-files", TestFindsSupportedFiles},
    {"skip-excluded-directories", TestSkipsExcludedDirectories},
    {"skip-symbolic-links", TestSkipsSymbolicLinks},
    {"empty-result", TestReturnsEmptyResult},
    {"reject-missing-root", TestRejectsMissingRoot},
    {"reject-file-root", TestRejectsFileRoot},
    {"reject-root-symlink", TestRejectsRootSymlink},
    {"reject-dangling-root-symlink", TestRejectsDanglingRootSymlink},
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
