#include <array>
#include <chrono>
#include <filesystem>
#include <fstream>
#include <iostream>
#include <string>
#include <string_view>

#include "cpp_defense/application/defense_session.hpp"

namespace {
namespace fs = std::filesystem;
using cpp_defense::CandidateSelectionMode;
using cpp_defense::DefenseSession;
using cpp_defense::DefenseSessionErrorType;
using cpp_defense::DefenseStatus;
using namespace std::chrono_literals;
constexpr int kSuccess = 0;
constexpr int kFailure = 1;

class TempDir {
 public:
  TempDir() {
    path_ = fs::temp_directory_path() /
            ("cpp-defense-session-" + std::to_string(
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
  std::ofstream out(path, std::ios::binary | std::ios::trunc);
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

struct Fixture {
  TempDir temp;
  fs::path app_root = temp.path() / "cppdefense";
  fs::path project = temp.path() / "lab";

  Fixture() {
    fs::create_directories(app_root);
    fs::create_directories(project);
    Write(project / "CMakeLists.txt",
          "cmake_minimum_required(VERSION 3.20)\n"
          "project(Lab LANGUAGES CXX)\n"
          "enable_testing()\n"
          "add_executable(lab_test main.cpp)\n"
          "add_test(NAME lab-test COMMAND lab_test)\n");
    Write(project / "main.cpp",
          "int Sum(int a, int b) {\n"
          "  int result = a + b;\n"
          "  result += 0;\n"
          "  return result;\n"
          "}\n\n"
          "int main() { return Sum(2, 3) == 5 ? 0 : 1; }\n");
  }
};

bool TestStart() {
  Fixture fixture;
  DefenseSession session(fixture.app_root);
  const auto result = session.Start(fixture.project, 1,
                                    CandidateSelectionMode::kFunctionsOnly,
                                    5min);
  if (!Expect(result.has_value(), "session starts")) return false;

  const std::string cached = Read(result->selected_entity.file_path);
  const std::string answer = Read(result->result_path);

  return Expect(result->selected_entity.name == "Sum", "largest function is selected") &&
         Expect(answer.find("int Sum(int a, int b)") != std::string::npos,
                "result file contains signature") &&
         Expect(answer.find("return result") == std::string::npos,
                "result file does not contain implementation") &&
         Expect(cached.find("return result") == std::string::npos,
                "cached implementation is masked") &&
         Expect(Read(fixture.project / "main.cpp").find("return result") != std::string::npos,
                "original project is untouched") &&
         Expect(session.status() == DefenseStatus::kActive,
                "session becomes active");
}

bool TestRetryThenSuccess() {
  Fixture fixture;
  DefenseSession session(fixture.app_root);
  const auto start = session.Start(fixture.project, 1,
                                   CandidateSelectionMode::kFunctionsOnly,
                                   5min);
  if (!Expect(start.has_value(), "session starts")) return false;

  Write(start->result_path,
        "int Sum(int a, int b) {\n"
        "  return a - b;\n"
        "}\n");
  const auto first_check = session.Check();
  if (!Expect(first_check.has_value(), "wrong solution is checked")) return false;

  const bool first_failed =
      first_check->build_result.configure.succeeded &&
      first_check->build_result.build.succeeded &&
      first_check->build_result.tests.attempted &&
      !first_check->build_result.tests.succeeded &&
      session.status() == DefenseStatus::kActive;

  Write(start->result_path,
        "int Sum(int a, int b) {\n"
        "  return a + b;\n"
        "}\n");
  const auto second_check = session.Check();
  if (!Expect(second_check.has_value(), "corrected solution is checked")) return false;

  const fs::path check_root = fixture.app_root / "cache/current/check";
  const std::string report = Read(start->defense_result_path);
  return Expect(first_failed, "failed tests keep session active") &&
         Expect(second_check->build_result.success(), "correct solution passes") &&
         Expect(session.status() == DefenseStatus::kSuccess,
                "successful check finishes session") &&
         Expect(!fs::exists(check_root), "temporary check workspace is cleaned") &&
         Expect(Read(start->selected_entity.file_path).find("return a + b") == std::string::npos,
                "user solution is never written into cached project") &&
         Expect(report.find("Attempts: 2") != std::string::npos,
                "final report stores attempt count") &&
         Expect(report.find("Result: success") != std::string::npos,
                "final report stores success status");
}

bool TestExpired() {
  Fixture fixture;
  DefenseSession session(fixture.app_root);
  const auto start = session.Start(fixture.project, 1,
                                   CandidateSelectionMode::kFunctionsOnly,
                                   0s);
  if (!Expect(start.has_value(), "zero-duration session starts")) return false;

  const auto check = session.Check();
  return Expect(!check.has_value(), "expired session cannot be checked") &&
         Expect(check.error().type == DefenseSessionErrorType::kSessionExpired,
                "expiration error is reported") &&
         Expect(session.status() == DefenseStatus::kExpired,
                "session status becomes expired");
}

bool TestNoActiveSession() {
  TempDir temp;
  const fs::path app_root = temp.path() / "cppdefense";
  fs::create_directories(app_root);
  DefenseSession session(app_root);
  const auto check = session.Check();
  return Expect(!check.has_value(), "check without start is rejected") &&
         Expect(check.error().type == DefenseSessionErrorType::kNoActiveSession,
                "no active session error is reported");
}

struct TestCase { std::string_view name; bool (*fn)(); };
constexpr std::array<TestCase, 4> kTests{{
    {"start", TestStart},
    {"retry-success", TestRetryThenSuccess},
    {"expired", TestExpired},
    {"no-active-session", TestNoActiveSession},
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
