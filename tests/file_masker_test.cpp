#include <array>
#include <chrono>
#include <cstddef>
#include <filesystem>
#include <fstream>
#include <iostream>
#include <string>
#include <string_view>

#include "cpp_defense/core/code_entity_info.hpp"
#include "cpp_defense/core/masker_error.hpp"
#include "cpp_defense/infrastructure/file_masker.hpp"

namespace {

namespace fs = std::filesystem;

using cpp_defense::CodeEntityInfo;
using cpp_defense::CodeEntityType;
using cpp_defense::FileMasker;
using cpp_defense::FileMaskerErrorType;

constexpr int kSuccessExitCode = 0;
constexpr int kFailureExitCode = 1;

class TemporaryDirectory {
 public:
  TemporaryDirectory() {
    const auto suffix =
        std::chrono::steady_clock::now().time_since_epoch().count();

    path_ = fs::temp_directory_path() /
            ("cpp-defense-file-masker-test-" + std::to_string(suffix));

    fs::create_directories(path_);
  }

  ~TemporaryDirectory() {
    std::error_code error_code;
    fs::remove_all(path_, error_code);
  }

  const fs::path& path() const noexcept {
    return path_;
  }

 private:
  fs::path path_;
};

bool Expect(bool condition, std::string_view message) {
  if (!condition) {
    std::cerr << "FAILED: " << message << '\n';
  }

  return condition;
}

void WriteFile(const fs::path& path, std::string_view contents) {
  std::ofstream output(path, std::ios::binary);
  output.write(contents.data(),
               static_cast<std::streamsize>(contents.size()));
}

std::string ReadFile(const fs::path& path) {
  std::ifstream input(path, std::ios::binary);
  return std::string(
      std::istreambuf_iterator<char>(input),
      std::istreambuf_iterator<char>());
}

CodeEntityInfo MakeEntity(const fs::path& path,
                          const std::string& source) {
  const std::size_t start = source.find("int Sum");
  const std::size_t body_start = source.find('{', start);
  const std::size_t body_end = source.find('}', body_start) + 1;

  return CodeEntityInfo{
      .type = CodeEntityType::kFunction,
      .name = "Sum",
      .file_path = path,
      .start_line = 1,
      .end_line = 4,
      .body_start_line = 1,
      .body_end_line = 4,
      .start_offset = start,
      .end_offset = body_end,
      .body_start_offset = body_start,
      .body_end_offset = body_end,
  };
}

bool TestMasksFunctionBody() {
  TemporaryDirectory temporary_directory;
  const fs::path file_path = temporary_directory.path() / "main.cpp";

  const std::string source =
      "int Sum(int a, int b) {\n"
      "  int result = a + b;\n"
      "  return result;\n"
      "}\n";

  WriteFile(file_path, source);

  const CodeEntityInfo entity = MakeEntity(file_path, source);
  const FileMasker masker;
  const auto result = masker.Mask(entity);

  if (!Expect(result.has_value(), "function body is masked")) {
    return false;
  }

  const std::string masked = ReadFile(file_path);

  return Expect(masked.size() == source.size(),
                "masking preserves file size") &&
         Expect(masked[entity.body_start_offset] == '{' &&
                    masked[entity.body_end_offset - 1] == '}',
                "masking preserves body braces") &&
         Expect(masked.find("a + b") == std::string::npos,
                "original implementation is removed") &&
         Expect(masked.find('\n') != std::string::npos,
                "line structure is preserved");
}

bool TestPreservesCrlfAndOffsets() {
  TemporaryDirectory temporary_directory;
  const fs::path file_path = temporary_directory.path() / "main.cpp";

  const std::string source =
      "int Sum(int a, int b) {\r\n"
      "  return a + b;\r\n"
      "}\r\n"
      "int value = 42;\r\n";

  WriteFile(file_path, source);

  const CodeEntityInfo entity = MakeEntity(file_path, source);
  const FileMasker masker;
  const auto result = masker.Mask(entity);

  if (!Expect(result.has_value(), "CRLF function body is masked")) {
    return false;
  }

  const std::string masked = ReadFile(file_path);

  return Expect(masked.size() == source.size(),
                "CRLF masking preserves byte offsets") &&
         Expect(masked.find("\r\n") != std::string::npos,
                "CRLF line endings are preserved") &&
         Expect(masked.substr(entity.end_offset) ==
                    source.substr(entity.end_offset),
                "source after the entity remains at the same offsets");
}

bool TestMasksTypeBodyAndPreservesSemicolon() {
  TemporaryDirectory temporary_directory;
  const fs::path file_path = temporary_directory.path() / "types.hpp";

  const std::string source =
      "class Player {\n"
      " public:\n"
      "  void Move();\n"
      "};\n";

  WriteFile(file_path, source);

  const std::size_t start = source.find("class Player");
  const std::size_t body_start = source.find('{', start);
  const std::size_t closing_brace = source.find('}', body_start);

  const CodeEntityInfo entity{
      .type = CodeEntityType::kClass,
      .name = "Player",
      .file_path = file_path,
      .start_line = 1,
      .end_line = 4,
      .body_start_line = 1,
      .body_end_line = 4,
      .start_offset = start,
      .end_offset = closing_brace + 2,
      .body_start_offset = body_start,
      .body_end_offset = closing_brace + 1,
  };

  const FileMasker masker;
  const auto result = masker.Mask(entity);

  if (!Expect(result.has_value(), "class body is masked")) {
    return false;
  }

  const std::string masked = ReadFile(file_path);

  return Expect(masked.find("void Move") == std::string::npos,
                "class members are removed") &&
         Expect(masked[entity.end_offset - 1] == ';',
                "trailing type semicolon is preserved");
}

bool TestRejectsInvalidRange() {
  TemporaryDirectory temporary_directory;
  const fs::path file_path = temporary_directory.path() / "main.cpp";

  const std::string source = "int Sum() {}\n";
  WriteFile(file_path, source);

  CodeEntityInfo entity = MakeEntity(file_path, source);
  entity.body_end_offset = source.size() + 10;

  const FileMasker masker;
  const auto result = masker.Mask(entity);

  return Expect(!result.has_value(),
                "invalid body range is rejected") &&
         Expect(result.error().type ==
                    FileMaskerErrorType::kInvalidEntityRange,
                "invalid range reports the correct error");
}

bool TestRejectsBraceMismatch() {
  TemporaryDirectory temporary_directory;
  const fs::path file_path = temporary_directory.path() / "main.cpp";

  const std::string source = "int Sum() []\n";
  WriteFile(file_path, source);

  const CodeEntityInfo entity{
      .type = CodeEntityType::kFunction,
      .name = "Sum",
      .file_path = file_path,
      .start_line = 1,
      .end_line = 1,
      .body_start_line = 1,
      .body_end_line = 1,
      .start_offset = 0,
      .end_offset = source.size() - 1,
      .body_start_offset = 10,
      .body_end_offset = 12,
  };

  const FileMasker masker;
  const auto result = masker.Mask(entity);

  return Expect(!result.has_value(),
                "body brace mismatch is rejected") &&
         Expect(result.error().type ==
                    FileMaskerErrorType::kBodyBoundaryMismatch,
                "brace mismatch reports the correct error");
}

struct TestCase {
  std::string_view name;
  bool (*function)();
};

constexpr std::array<TestCase, 5> kTestCases{{
    {"mask-function-body", TestMasksFunctionBody},
    {"preserve-crlf-offsets", TestPreservesCrlfAndOffsets},
    {"mask-type-body", TestMasksTypeBodyAndPreservesSemicolon},
    {"reject-invalid-range", TestRejectsInvalidRange},
    {"reject-brace-mismatch", TestRejectsBraceMismatch},
}};

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

  std::cerr << "Unknown test case: " << requested_test << '\n';
  return kFailureExitCode;
}
