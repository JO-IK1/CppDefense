#include "cpp_defense/ui/cli_app.hpp"

#include <iostream>
#include <ostream>

namespace cpp_defense {
namespace {

constexpr int kSuccessExitCode = 0;
constexpr int kErrorExitCode = 1;

}  // namespace

CliApp::CliApp() : CliApp(std::cin, std::cout, std::cerr) {}

CliApp::CliApp(std::istream& input, std::ostream& output,
               std::ostream& error_output)
    : input_(input), output_(output), error_output_(error_output) {}

int CliApp::Run(int argc, char* argv[]) {
  PrintHeader();

  const CommandParseResult parse_result = command_parser_.Parse(argc, argv);
  if (parse_result.help_requested()) {
    PrintUsage(output_);
    return kSuccessExitCode;
  }

  if (!parse_result.ok()) {
    PrintParseError(parse_result);
    PrintUsage(error_output_);
    return kErrorExitCode;
  }

  PrintStartMessage(parse_result.options());
  return kSuccessExitCode;
}

void CliApp::PrintHeader() const {
  output_ << "CppDefense CLI\n";
  output_ << "Version: 0.2.0\n";
}

void CliApp::PrintUsage(std::ostream& output) const {
  output << CommandParser::UsageText();
}

void CliApp::PrintParseError(const CommandParseResult& result) const {
  error_output_ << "Error: " << result.message() << "\n\n";
}

void CliApp::PrintStartMessage(const CliOptions& options) const {
  output_ << "Project path: " << options.project_path() << '\n';
  output_ << "Function candidates: " << options.function_count() << '\n';
  output_ << "Timer: " << options.timer_minutes() << " minutes\n";
  output_ << "Mode: "
          << (options.functions_only() ? "functions only" : "all supported code")
          << '\n';
}

}  // namespace cpp_defense
