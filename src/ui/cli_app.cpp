#include "cpp_defense/ui/cli_app.hpp"

#include <chrono>
#include <filesystem>
#include <iomanip>
#include <iostream>
#include <ostream>
#include <string>
#include <string_view>
#include <utility>

namespace cpp_defense {
namespace {

constexpr int kSuccessExitCode = 0;
constexpr int kErrorExitCode = 1;

std::string_view EntityTypeName(CodeEntityType type) {
  switch (type) {
    case CodeEntityType::kFunction:
      return "function";
    case CodeEntityType::kClass:
      return "class";
    case CodeEntityType::kStruct:
      return "struct";
    case CodeEntityType::kEnumClass:
      return "enum class";
  }
  return "unknown";
}

std::string_view DefenseStatusName(DefenseStatus status) {
  switch (status) {
    case DefenseStatus::kIdle:
      return "idle";
    case DefenseStatus::kPreparing:
      return "preparing";
    case DefenseStatus::kWorkspaceReady:
      return "workspace ready";
    case DefenseStatus::kActive:
      return "active";
    case DefenseStatus::kChecking:
      return "checking";
    case DefenseStatus::kSuccess:
      return "success";
    case DefenseStatus::kFailed:
      return "failed";
    case DefenseStatus::kExpired:
      return "expired";
    case DefenseStatus::kError:
      return "error";
  }
  return "unknown";
}

void PrintStep(std::ostream& output,
               std::string_view name,
               const BuildStepResult& step) {
  if (!step.attempted) {
    output << name << ": skipped\n";
    return;
  }

  output << name << ": " << (step.succeeded ? "OK" : "FAILED") << '\n';

  if (!step.succeeded && !step.output.empty()) {
    output << "\n" << step.output;
    if (step.output.back() != '\n') {
      output << '\n';
    }
  }
}

void PrintRemaining(std::ostream& output, std::chrono::seconds remaining) {
  const auto total_seconds = remaining.count();
  const auto minutes = total_seconds / 60;
  const auto seconds = total_seconds % 60;

  output << "Time remaining: " << minutes << ':'
         << std::setfill('0') << std::setw(2) << seconds
         << std::setfill(' ') << '\n';
}

}  // namespace

CliApp::CliApp() : CliApp(std::cin, std::cout, std::cerr, ".") {}

CliApp::CliApp(std::istream& input, std::ostream& output,
               std::ostream& error_output)
    : CliApp(input, output, error_output, ".") {}

CliApp::CliApp(std::istream& input, std::ostream& output,
               std::ostream& error_output,
               std::filesystem::path cpp_defense_root_path)
    : defense_session_(std::move(cpp_defense_root_path)),
      input_(input),
      output_(output),
      error_output_(error_output) {}

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

  CliOptions options = parse_result.options();
  PrintStartMessage(options);
  return RunInteractiveLoop(options);
}

int CliApp::RunInteractiveLoop(CliOptions& options) {
  output_ << "Type -h or --help to show help message.\n"
          << "Type -e or --exit to close the application.\n";

  std::string command;
  while (true) {
    output_ << "> " << std::flush;

    if (!std::getline(input_, command)) {
      error_output_ << "Error: input stream was closed.\n";
      return kErrorExitCode;
    }

    if (defense_session_.ExpireIfNeeded()) {
      output_ << "Defense time expired.\n";
      const auto finish_result = defense_session_.Finish();
      if (finish_result && defense_session_.workspace()) {
        output_ << "Result saved: "
                << defense_session_.workspace()->defense_result_path << '\n';
      }
    }

    const InteractiveParseResult result = command_parser_.ParseInteractive(command, options);
    if (!result.ok()) {
      error_output_ << "Error: " << result.message() << '\n';
      continue;
    }

    switch (result.type()) {
      case InteractiveCommandType::kEmpty:
        break;
      case InteractiveCommandType::kHelp:
        PrintUsage(output_);
        break;
      case InteractiveCommandType::kExit: {
        if (defense_session_.selected_entity()) {
          const auto finish_result = defense_session_.Finish();
          if (!finish_result) {
            error_output_ << "Error: " << finish_result.error().FullMessage()
                          << '\n';
          } else if (defense_session_.workspace()) {
            output_ << "Result saved: "
                    << defense_session_.workspace()->defense_result_path
                    << '\n';
          }
        }
        return kSuccessExitCode;
      }
      case InteractiveCommandType::kSetPath:
        output_ << "Project path selected: " << options.project_path() << '\n';
        break;
      case InteractiveCommandType::kSetFunctionCount:
        output_ << "Function candidates: " << options.function_count() << '\n';
        break;
      case InteractiveCommandType::kSetTimer:
        output_ << "Timer: " << options.timer_minutes() << " minutes\n";
        break;
      case InteractiveCommandType::kFunctionsOnly:
      case InteractiveCommandType::kAll:
        output_ << "Mode: "
                << (options.functions_only() ? "functions only"
                                             : "all supported code")
                << '\n';
        break;

      case InteractiveCommandType::kStart: {
        if (options.project_path().empty()) {
          error_output_
              << "Select a project path first using -p <project_path>.\n";
          break;
        }
        output_ << "Starting defense...\n";

        const CandidateSelectionMode mode =
            options.functions_only()
                ? CandidateSelectionMode::kFunctionsOnly
                : CandidateSelectionMode::kAll;

        const auto start_result = defense_session_.Start(
            options.project_path(),
            static_cast<std::size_t>(options.function_count()),
            mode,
            std::chrono::minutes(options.timer_minutes()));

        if (!start_result) {
          error_output_ << "Error: " << start_result.error().FullMessage()
                        << '\n';
          break;
        }

        output_ << "Defense started.\n"
                << "Source files found: " << start_result->source_file_count << '\n'
                << "Entities found: " << start_result->entity_count << '\n'
                << "Candidates retained: " << start_result->candidate_count << '\n'
                << "Selected: ["
                << EntityTypeName(start_result->selected_entity.type) << "] "
                << start_result->selected_entity.name << '\n'
                << "Cached project: " << start_result->cached_project_path << '\n'
                << "Edit this file: " << start_result->result_path << '\n'
                << "Final report: " << start_result->defense_result_path << '\n';
        PrintRemaining(output_, defense_session_.remaining_time());
        break;
      }

      case InteractiveCommandType::kInfo: {
        if (!defense_session_.selected_entity() || !defense_session_.workspace()) {
          error_output_ << "Error: there is no defense session.\n";
          break;
        }
        const CodeEntityInfo& entity = *defense_session_.selected_entity();
        output_ << "Status: " << DefenseStatusName(defense_session_.status()) << '\n'
                << "Selected: [" << EntityTypeName(entity.type) << "] "
                << entity.name << '\n'
                << "File: " << entity.file_path << '\n'
                << "Lines: " << entity.start_line << '-' << entity.end_line << '\n'
                << "Edit: " << defense_session_.workspace()->result_path << '\n'
                << "Attempts: " << defense_session_.attempts() << '\n';
        break;
      }

      case InteractiveCommandType::kTime:
        if (defense_session_.status() == DefenseStatus::kActive) {
          PrintRemaining(output_, defense_session_.remaining_time());
        } else {
          output_ << "Defense status: "
                  << DefenseStatusName(defense_session_.status()) << '\n';
        }
        break;

      case InteractiveCommandType::kCheck: {
        output_ << "Checking solution...\n";

        const auto check_result = defense_session_.Check();
        if (!check_result) {
          error_output_ << "Error: " << check_result.error().FullMessage()
                        << '\n';
          break;
        }

        output_ << "Attempt: " << check_result->attempt << '\n';
        PrintStep(output_, "Configure", check_result->build_result.configure);
        PrintStep(output_, "Build", check_result->build_result.build);
        PrintStep(output_, "Tests", check_result->build_result.tests);

        if (check_result->status == DefenseStatus::kExpired) {
          output_ << "Defense time expired during the check.\n";
        } else if (check_result->build_result.success()) {
          output_ << "Defense passed.\n";
          if (defense_session_.workspace()) {
            output_ << "Result saved: "
                    << defense_session_.workspace()->defense_result_path
                    << '\n';
          }
        } else {
          output_ << "Check failed. Fix result.txt and run --check again.\n";
          PrintRemaining(output_, defense_session_.remaining_time());
        }
        break;
      }
    }
  }
}

void CliApp::PrintHeader() const {
  output_ << "CppDefense CLI\n";
  output_ << "Version: " << CPP_DEFENSE_VERSION << '\n';
}

void CliApp::PrintUsage(std::ostream& output) const {
  output << CommandParser::UsageText();
}

void CliApp::PrintParseError(const CommandParseResult& result) const {
  error_output_ << "Error: " << result.message() << "\n\n";
}

void CliApp::PrintStartMessage(const CliOptions& options) const {
  if (options.project_path().empty()) {
    output_ << "Project path: not selected\n";
  } else {
    output_ << "Project path: " << options.project_path() << '\n';
  }
  output_ << "Function candidates: " << options.function_count() << '\n';
  output_ << "Timer: " << options.timer_minutes() << " minutes\n";
  output_ << "Mode: "
          << (options.functions_only() ? "functions only" : "all supported code")
          << "\n\n";
}

}  // namespace cpp_defense
