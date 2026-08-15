#include <chrono>
#include <filesystem>
#include <fstream>
#include <iostream>
#include <string>
#include <string_view>

#include "cpp_defense/core/source_file_error.hpp"
#include "cpp_defense/infrastructure/source_file_repository.hpp"

namespace {

namespace fs = std::filesystem;

using cpp_defense::SourceFileErrorType;
using cpp_defense::SourceFileRepository;

constexpr int kSuccessExitCode = 0;
constexpr int kFailureExitCode = 1;

class TemporaryDirectory {
 public:
  TemporaryDirectory() {
    const auto timestamp =
        std::chrono::steady_clock::now()
            .time_since_epoch()
            .count();

    path_ =
        fs::temp_directory_path() /
        ("cpp-defense-source-file-repository-test-" +
         std::to_string(timestamp));

    fs::create_directories(path_);
  }

  ~TemporaryDirectory() {
    std::error_code error_code;
    fs::remove_all(path_, error_code);
  }

  const fs::path& path() const {
    return path_;
  }

 private:
  fs::path path_;
};

bool Expect(bool condition,
            std::string_view message) {
  if (condition) {
    return true;
  }

  std::cerr << "FAILED: " << message << '\n';
  return false;
}

void WriteFile(const fs::path& path,
               std::string_view contents) {
  fs::create_directories(path.parent_path());

  std::ofstream output(path, std::ios::binary);
  output.write(contents.data(),
               static_cast<std::streamsize>(
                   contents.size()));
}

bool TestReadsFile() {
  TemporaryDirectory temporary_directory;

  const fs::path file_path =
      temporary_directory.path() / "main.cpp";

  WriteFile(file_path, "int main() {}\n");

  const SourceFileRepository repository;
  const auto result = repository.ReadFile(file_path);

  return Expect(
      result.has_value() &&
          *result == "int main() {}\n",
      "source file contents are read exactly");
}

bool TestReadsEmptyFile() {
  TemporaryDirectory temporary_directory;

  const fs::path file_path =
      temporary_directory.path() / "empty.cpp";

  WriteFile(file_path, "");

  const SourceFileRepository repository;
  const auto result = repository.ReadFile(file_path);

  return Expect(
      result.has_value() && result->empty(),
      "empty source file is read successfully");
}

bool TestPreservesCrlf() {
  TemporaryDirectory temporary_directory;

  const fs::path file_path =
      temporary_directory.path() / "crlf.cpp";

  const std::string contents =
      "int Foo() {\r\n"
      "  return 1;\r\n"
      "}\r\n";

  WriteFile(file_path, contents);

  const SourceFileRepository repository;
  const auto result = repository.ReadFile(file_path);

  return Expect(
      result.has_value() &&
          *result == contents,
      "CRLF line endings are preserved exactly");
}

bool TestRejectsMissingFile() {
  TemporaryDirectory temporary_directory;

  const fs::path file_path =
      temporary_directory.path() / "missing.cpp";

  const SourceFileRepository repository;
  const auto result = repository.ReadFile(file_path);

  return Expect(
      !result.has_value() &&
          result.error().type ==
              SourceFileErrorType::kOpenFailed,
      "missing source file produces open error");
}

struct TestCase {
  std::string_view name;
  bool (*function)();
};

constexpr TestCase kTestCases[] = {
    {"read-file", TestReadsFile},
    {"read-empty-file", TestReadsEmptyFile},
    {"preserve-crlf", TestPreservesCrlf},
    {"reject-missing-file", TestRejectsMissingFile},
};

}  // namespace

int main(int argc, char* argv[]) {
  if (argc != 2) {
    std::cerr << "Expected test case name\n";
    return kFailureExitCode;
  }

  const std::string_view requested_test = argv[1];

  for (const auto& test_case : kTestCases) {
    if (test_case.name == requested_test) {
      return test_case.function()
                 ? kSuccessExitCode
                 : kFailureExitCode;
    }
  }

  std::cerr << "Unknown test case: "
            << requested_test << '\n';

  return kFailureExitCode;
}
