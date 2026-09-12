#include <array>
#include <chrono>
#include <filesystem>
#include <fstream>
#include <iostream>
#include <sstream>
#include <string>
#include <string_view>

#include "cpp_defense/ui/cli_app.hpp"
#include "cpp_defense/ui/command_parser.hpp"

namespace {
namespace fs = std::filesystem;
using cpp_defense::CliApp;
using cpp_defense::CliOptions;
using cpp_defense::CommandParseStatus;
using cpp_defense::CommandParser;
using cpp_defense::InteractiveCommandType;

class TempDir {
 public:
  TempDir() {
    path_ = fs::temp_directory_path() /
            ("cpp-defense-ui-" + std::to_string(
                std::chrono::steady_clock::now().time_since_epoch().count()));
    fs::create_directories(path_);
  }
  ~TempDir() { std::error_code error; fs::remove_all(path_, error); }
  const fs::path& path() const { return path_; }

 private:
  fs::path path_;
};

bool Expect(bool condition, std::string_view message) {
  if (!condition) std::cerr << "FAILED: " << message << '\n';
  return condition;
}

void Write(const fs::path& path, std::string_view contents) {
  fs::create_directories(path.parent_path());
  std::ofstream output(path);
  output << contents;
}

std::string Read(const fs::path& path) {
  std::ifstream input(path);
  return {std::istreambuf_iterator<char>(input),
          std::istreambuf_iterator<char>()};
}

bool TestCommandParser() {
  TempDir temp;
  std::string program = "cpp-defense";
  std::string path = temp.path().string();
  std::string count = "7";
  char* arguments[] = {program.data(), path.data(),
                       const_cast<char*>("--functions"), count.data(),
                       const_cast<char*>("--all")};

  const CommandParser parser;
  const auto startup = parser.Parse(5, arguments);
  CliOptions options;
  const auto interactive = parser.ParseInteractive("--timer 12", options);
  const auto invalid = parser.ParseInteractive("--timer 0", options);
  return Expect(startup.ok(), "valid startup options parse") &&
         Expect(startup.options().function_count() == 7,
                "startup candidate count is stored") &&
         Expect(!startup.options().functions_only(), "all mode is stored") &&
         Expect(interactive.ok() &&
                    interactive.type() == InteractiveCommandType::kSetTimer,
                "interactive timer command parses") &&
         Expect(options.timer_minutes() == 12, "interactive timer is stored") &&
         Expect(!invalid.ok(), "invalid interactive value is rejected") &&
         Expect(startup.status() == CommandParseStatus::kSuccess,
                "startup status is success");
}

bool TestEofFinalizesSession() {
  TempDir temp;
  const fs::path project = temp.path() / "project";
  const fs::path runtime = temp.path() / "runtime";
  Write(project / "main.cpp",
        "int Sum(int a, int b) {\n"
        "  return a + b;\n"
        "}\n");

  std::istringstream input("start\n");
  std::ostringstream output;
  std::ostringstream errors;
  CliApp app(input, output, errors, runtime);

  std::string program = "cpp-defense";
  std::string project_argument = project.string();
  char* arguments[] = {program.data(), project_argument.data()};
  const int exit_code = app.Run(2, arguments);
  fs::path report;
  for (const auto& entry : fs::directory_iterator(runtime / "cache")) {
    if (entry.is_directory()) report = entry.path() / "defense_result.txt";
  }
  return Expect(exit_code == 0, "EOF exits cleanly") &&
         Expect(fs::is_regular_file(report), "EOF saves final report") &&
         Expect(Read(report).find("Result: failed") != std::string::npos,
                "EOF marks active defense as failed") &&
         Expect(errors.str().empty(), "EOF is not reported as an input error");
}

struct TestCase { std::string_view name; bool (*run)(); };
constexpr std::array<TestCase, 2> kTests{{
    {"command-parser", TestCommandParser},
    {"eof-finalizes", TestEofFinalizesSession},
}};
}  // namespace

int main(int argc, char* argv[]) {
  if (argc != 2) return 2;
  for (const auto& test : kTests) {
    if (test.name == argv[1]) return test.run() ? 0 : 1;
  }
  return 2;
}
