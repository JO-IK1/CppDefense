#include <array>
#include <chrono>
#include <filesystem>
#include <fstream>
#include <iostream>
#include <string>
#include <string_view>

#include "cpp_defense/infrastructure/file_patcher.hpp"

namespace {
namespace fs = std::filesystem;
using cpp_defense::CodeEntityInfo;
using cpp_defense::CodeEntityType;
using cpp_defense::FilePatcher;
using cpp_defense::FilePatcherErrorType;
constexpr int kSuccess = 0;
constexpr int kFailure = 1;

class TempDir {
 public:
  TempDir() {
    path_ = fs::temp_directory_path() /
            ("cpp-defense-patcher-" + std::to_string(
                std::chrono::steady_clock::now().time_since_epoch().count()));
    fs::create_directories(path_);
  }
  ~TempDir() { std::error_code ec; fs::remove_all(path_, ec); }
  const fs::path& path() const { return path_; }
 private:
  fs::path path_;
};

void Write(const fs::path& path, std::string_view text) {
  fs::create_directories(path.parent_path());
  std::ofstream out(path, std::ios::binary);
  out << text;
}

std::string Read(const fs::path& path) {
  std::ifstream in(path, std::ios::binary);
  return {std::istreambuf_iterator<char>(in), std::istreambuf_iterator<char>()};
}

bool Expect(bool condition, std::string_view message) {
  if (!condition) std::cerr << "FAILED: " << message << '\n';
  return condition;
}

bool TestPatch() {
  TempDir temp;
  const fs::path cached = temp.path() / "cached";
  const fs::path check = temp.path() / "check";
  const fs::path cached_file = cached / "src/main.cpp";
  const fs::path check_file = check / "src/main.cpp";
  const std::string masked = "int Sum(int a, int b) {       }\nint x = 1;\n";
  Write(cached_file, masked);
  Write(check_file, masked);

  const std::size_t start = masked.find("int Sum");
  const std::size_t body_start = masked.find('{', start);
  const std::size_t end = masked.find('}', start) + 1;
  CodeEntityInfo entity{.type = CodeEntityType::kFunction,
                        .name = "Sum", .file_path = cached_file,
                        .start_offset = start, .end_offset = end,
                        .body_start_offset = body_start,
                        .body_end_offset = end};

  FilePatcher patcher;
  const std::string replacement = " return a + b; ";
  const auto result = patcher.Patch(entity, cached, check, replacement);

  return Expect(result.has_value(), "patch succeeds") &&
         Expect(Read(cached_file) == masked, "cached project remains masked") &&
         Expect(Read(check_file).find("int Sum(int a, int b) {") !=
                    std::string::npos,
                "original declaration is preserved") &&
         Expect(Read(check_file).find(replacement) != std::string::npos,
                "check copy contains solution") &&
         Expect(Read(check_file).find("int x = 1;") != std::string::npos,
                "source after entity is preserved");
}

bool TestRejectsEscapingBody() {
  TempDir temp;
  const fs::path cached = temp.path() / "cached";
  const fs::path check = temp.path() / "check";
  const fs::path cached_file = cached / "main.cpp";
  const fs::path check_file = check / "main.cpp";
  const std::string masked = "int Sum() {     }\nint untouched = 1;\n";
  Write(cached_file, masked);
  Write(check_file, masked);

  const std::size_t body_start = masked.find('{');
  const std::size_t body_end = masked.find('}') + 1;
  const CodeEntityInfo entity{
      .type = CodeEntityType::kFunction,
      .name = "Sum",
      .file_path = cached_file,
      .start_offset = 0,
      .end_offset = body_end,
      .body_start_offset = body_start,
      .body_end_offset = body_end,
  };

  FilePatcher patcher;
  const auto result = patcher.Patch(
      entity, cached, check, "}\nint injected = 1;\n{");
  return Expect(!result.has_value(), "body escape is rejected") &&
         Expect(result.error().type ==
                    FilePatcherErrorType::kInvalidReplacement,
                "body escape reports invalid replacement") &&
         Expect(Read(check_file) == masked,
                "invalid replacement does not modify check source");
}

bool TestOutsideProject() {
  TempDir temp;
  const fs::path cached = temp.path() / "cached";
  const fs::path check = temp.path() / "check";
  const fs::path outside = temp.path() / "outside.cpp";
  Write(outside, "int Foo() {}\n");
  CodeEntityInfo entity{.type = CodeEntityType::kFunction,
                        .name = "Foo", .file_path = outside,
                        .start_offset = 0, .end_offset = 12};
  FilePatcher patcher;
  const auto result = patcher.Patch(entity, cached, check, "int Foo() {}");
  return Expect(!result.has_value(), "outside entity is rejected") &&
         Expect(result.error().type ==
                    FilePatcherErrorType::kEntityOutsideCachedProject,
                "outside error type is correct");
}

struct TestCase { std::string_view name; bool (*fn)(); };
constexpr std::array<TestCase, 3> kTests{{
    {"patch", TestPatch},
    {"reject-body-escape", TestRejectsEscapingBody},
    {"outside-project", TestOutsideProject},
}};
}

int main(int argc, char* argv[]) {
  if (argc != 2) return kFailure;
  for (const auto& test : kTests) {
    if (test.name == argv[1]) return test.fn() ? kSuccess : kFailure;
  }
  std::cerr << "Unknown test case: " << argv[1] << '\n';
  return kFailure;
}
