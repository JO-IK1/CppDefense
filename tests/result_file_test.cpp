#include <array>
#include <chrono>
#include <cstddef>
#include <filesystem>
#include <fstream>
#include <iostream>
#include <string>
#include <string_view>

#include "cpp_defense/core/code_entity_info.hpp"
#include "cpp_defense/core/result_error.hpp"
#include "cpp_defense/infrastructure/result_file.hpp"

namespace {

namespace fs = std::filesystem;

using cpp_defense::CodeEntityInfo;
using cpp_defense::CodeEntityType;
using cpp_defense::ResultFile;
using cpp_defense::ResultFileErrorType;

constexpr int kSuccessExitCode = 0;
constexpr int kFailureExitCode = 1;

class TemporaryDirectory {
 public:
  TemporaryDirectory() {
    const auto suffix =
        std::chrono::steady_clock::now().time_since_epoch().count();

    path_ = fs::temp_directory_path() /
            ("cpp-defense-result-file-test-" + std::to_string(suffix));

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

CodeEntityInfo MakeEntity(const fs::path& path,
                          CodeEntityType type,
                          std::size_t start,
                          std::size_t body_start,
                          std::size_t body_end,
                          std::size_t end) {
  return CodeEntityInfo{
      .type = type,
      .name = "Entity",
      .file_path = path,
      .start_line = 1,
      .end_line = 1,
      .body_start_line = 1,
      .body_end_line = 1,
      .start_offset = start,
      .end_offset = end,
      .body_start_offset = body_start,
      .body_end_offset = body_end,
  };
}

bool TestCreatesFunctionSignatureTemplate() {
  TemporaryDirectory temporary_directory;

  const fs::path source_path = temporary_directory.path() / "main.cpp";
  const fs::path result_path = temporary_directory.path() / "result.txt";

  const std::string source =
      "int Sum(int a, int b) {\n"
      "  return a + b;\n"
      "}\n";

  WriteFile(source_path, source);

  const std::size_t start = source.find("int Sum");
  const std::size_t body_start = source.find('{', start);
  const std::size_t body_end = source.find('}', body_start) + 1;

  const CodeEntityInfo entity = MakeEntity(
      source_path, CodeEntityType::kFunction,
      start, body_start, body_end, body_end);

  const ResultFile result_file;
  const auto create_result = result_file.Create(entity, result_path);

  if (!Expect(create_result.has_value(),
              "function result file is created")) {
    return false;
  }

  const auto contents = result_file.Read(result_path);

  return Expect(contents.has_value(),
                "created result file can be read") &&
         Expect(*contents == "int Sum(int a, int b) {\n\n}\n",
                "result file contains the original signature and empty body");
}

bool TestPreservesMultilineSignature() {
  TemporaryDirectory temporary_directory;

  const fs::path source_path = temporary_directory.path() / "main.cpp";
  const fs::path result_path = temporary_directory.path() / "result.txt";

  const std::string source =
      "int Sum(\n"
      "    int a,\n"
      "    int b)\n"
      "{\n"
      "  return a + b;\n"
      "}\n";

  WriteFile(source_path, source);

  const std::size_t start = source.find("int Sum");
  const std::size_t body_start = source.find('{', start);
  const std::size_t body_end = source.find('}', body_start) + 1;

  const CodeEntityInfo entity = MakeEntity(
      source_path, CodeEntityType::kFunction,
      start, body_start, body_end, body_end);

  const ResultFile result_file;
  const auto result = result_file.Create(entity, result_path);

  if (!Expect(result.has_value(),
              "multiline result file is created")) {
    return false;
  }

  const auto contents = result_file.Read(result_path);

  return Expect(contents.has_value(),
                "multiline result file can be read") &&
         Expect(*contents ==
                    "int Sum(\n"
                    "    int a,\n"
                    "    int b)\n"
                    "{\n\n}\n",
                "multiline signature is preserved exactly");
}

bool TestPreservesTypeSemicolon() {
  TemporaryDirectory temporary_directory;

  const fs::path source_path = temporary_directory.path() / "types.hpp";
  const fs::path result_path = temporary_directory.path() / "result.txt";

  const std::string source =
      "struct Config {\n"
      "  int width;\n"
      "};\n";

  WriteFile(source_path, source);

  const std::size_t start = source.find("struct Config");
  const std::size_t body_start = source.find('{', start);
  const std::size_t closing_brace = source.find('}', body_start);
  const std::size_t body_end = closing_brace + 1;
  const std::size_t end = source.find(';', closing_brace) + 1;

  const CodeEntityInfo entity = MakeEntity(
      source_path, CodeEntityType::kStruct,
      start, body_start, body_end, end);

  const ResultFile result_file;
  const auto result = result_file.Create(entity, result_path);

  if (!Expect(result.has_value(), "struct result file is created")) {
    return false;
  }

  const auto contents = result_file.Read(result_path);

  return Expect(contents.has_value(),
                "struct result file can be read") &&
         Expect(*contents == "struct Config {\n\n};\n",
                "type template preserves its trailing semicolon");
}

bool TestRejectsInvalidRange() {
  TemporaryDirectory temporary_directory;

  const fs::path source_path = temporary_directory.path() / "main.cpp";
  const fs::path result_path = temporary_directory.path() / "result.txt";

  const std::string source = "int Sum() {}\n";
  WriteFile(source_path, source);

  const CodeEntityInfo entity = MakeEntity(
      source_path, CodeEntityType::kFunction,
      10, 5, 7, 8);

  const ResultFile result_file;
  const auto result = result_file.Create(entity, result_path);

  return Expect(!result.has_value(),
                "invalid entity range is rejected") &&
         Expect(result.error().type ==
                    ResultFileErrorType::kInvalidEntityRange,
                "invalid entity range reports the correct error");
}

bool TestReadRejectsMissingFile() {
  TemporaryDirectory temporary_directory;

  const fs::path result_path = temporary_directory.path() / "missing.txt";

  const ResultFile result_file;
  const auto result = result_file.Read(result_path);

  return Expect(!result.has_value(),
                "missing result file is rejected") &&
         Expect(result.error().type == ResultFileErrorType::kOpenFailed,
                "missing result file reports open error");
}

struct TestCase {
  std::string_view name;
  bool (*function)();
};

constexpr std::array<TestCase, 5> kTestCases{{
    {"function-signature", TestCreatesFunctionSignatureTemplate},
    {"multiline-signature", TestPreservesMultilineSignature},
    {"type-semicolon", TestPreservesTypeSemicolon},
    {"reject-invalid-range", TestRejectsInvalidRange},
    {"read-missing-file", TestReadRejectsMissingFile},
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
