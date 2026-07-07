#include "cpp_defense/ui/command_parser.hpp"

#include <charconv>
#include <filesystem>
#include <limits>
#include <string>
#include <system_error>
#include <utility>

namespace cpp_defense {
namespace {

constexpr int kMaxFunctionCount = 50;
constexpr int kMaxTimerMinutes = 180;

bool IsOption(std::string_view argument) noexcept {
  return argument.starts_with('-');
}

}  // namespace

CommandParseResult CommandParseResult::Success(CliOptions options) {
  return CommandParseResult(CommandParseStatus::kSuccess, std::move(options), "");
}

CommandParseResult CommandParseResult::HelpRequested() {
  return CommandParseResult(CommandParseStatus::kHelpRequested, CliOptions{}, "");
}

CommandParseResult CommandParseResult::Error(CommandParseStatus status,
                                             std::string message) {
  return CommandParseResult(status, CliOptions{}, std::move(message));
}

bool CommandParseResult::ok() const noexcept {
  return status_ == CommandParseStatus::kSuccess;
}

bool CommandParseResult::help_requested() const noexcept {
  return status_ == CommandParseStatus::kHelpRequested;
}

CommandParseResult::CommandParseResult(CommandParseStatus status,
                                       CliOptions options, std::string message)
    : status_(status),
      options_(std::move(options)),
      message_(std::move(message)) {}

CommandParseResult CommandParser::Parse(int argc, char* argv[]) const {
  if (argc <= 1) {
    return CommandParseResult::Error(
        CommandParseStatus::kMissingProjectPath,
        "Project path is required.");
  }

  CliOptions options;
  bool project_path_set = false;

  for (int index = 1; index < argc; ++index) {
    const std::string_view argument(argv[index]);

    if (IsHelpArgument(argument)) {
      return CommandParseResult::HelpRequested();
    }

    if (IsFunctionCountOption(argument)) {
      if (index + 1 >= argc) {
        return CommandParseResult::Error(
            CommandParseStatus::kMissingOptionValue,
            "Function count option requires a value.");
      }

      int function_count = 0;
      const std::string_view value(argv[++index]);
      if (!TryParsePositiveInt(value, &function_count) ||
          function_count > kMaxFunctionCount) {
        return CommandParseResult::Error(
            CommandParseStatus::kInvalidFunctionCount,
            "Function count must be an integer from 1 to 50.");
      }

      options.set_function_count(function_count);
      continue;
    }

    if (IsTimerOption(argument)) {
      if (index + 1 >= argc) {
        return CommandParseResult::Error(CommandParseStatus::kMissingOptionValue,
                                         "Timer option requires a value.");
      }

      int timer_minutes = 0;
      const std::string_view value(argv[++index]);
      if (!TryParsePositiveInt(value, &timer_minutes) ||
          timer_minutes > kMaxTimerMinutes) {
        return CommandParseResult::Error(
            CommandParseStatus::kInvalidTimer,
            "Timer must be an integer from 1 to 180 minutes.");
      }

      options.set_timer_minutes(timer_minutes);
      continue;
    }

    if (argument == "--all") {
      options.set_functions_only(false);
      continue;
    }

    if (argument == "--functions-only") {
      options.set_functions_only(true);
      continue;
    }

    if (IsOption(argument)) {
      return CommandParseResult::Error(
          CommandParseStatus::kUnknownArgument,
          "Unknown argument: " + std::string(argument));
    }

    if (project_path_set) {
      return CommandParseResult::Error(
          CommandParseStatus::kUnknownArgument,
          "Only one project path can be provided.");
    }

    options.set_project_path(std::filesystem::path(argument));
    project_path_set = true;
  }

  if (!project_path_set) {
    return CommandParseResult::Error(CommandParseStatus::kMissingProjectPath,
                                     "Project path is required.");
  }

  std::error_code error_code;
  const bool exists = std::filesystem::exists(options.project_path(), error_code);
  if (error_code || !exists) {
    return CommandParseResult::Error(
        CommandParseStatus::kProjectPathDoesNotExist,
        "Project path does not exist: " + options.project_path().string());
  }

  const bool is_directory =
      std::filesystem::is_directory(options.project_path(), error_code);
  if (error_code || !is_directory) {
    return CommandParseResult::Error(
        CommandParseStatus::kProjectPathIsNotDirectory,
        "Project path is not a directory: " + options.project_path().string());
  }

  return CommandParseResult::Success(std::move(options));
}

std::string_view CommandParser::ProgramName() noexcept { return "cpp-defense"; }

std::string CommandParser::UsageText() {
  return "Usage: cpp-defense <project_path> [options]\n"
         "\n"
         "Options:\n"
         "  -h, --help                 Show this help message.\n"
         "  -n, --functions <count>    Number of candidate functions. Default: 5.\n"
         "  -t, --timer <minutes>      Defense timer in minutes. Default: 5.\n"
         "      --functions-only       Pick only functions. Default behavior.\n"
         "      --all                  Allow all supported code fragments.\n";
}

bool CommandParser::IsHelpArgument(std::string_view argument) noexcept {
  return argument == "-h" || argument == "--help";
}

bool CommandParser::IsFunctionCountOption(std::string_view argument) noexcept {
  return argument == "-n" || argument == "--functions";
}

bool CommandParser::IsTimerOption(std::string_view argument) noexcept {
  return argument == "-t" || argument == "--timer";
}

bool CommandParser::TryParsePositiveInt(std::string_view value,
                                        int* result) noexcept {
  if (result == nullptr || value.empty()) {
    return false;
  }

  int parsed_value = 0;
  const char* first = value.data();
  const char* last = value.data() + value.size();
  const auto [ptr, error_code] = std::from_chars(first, last, parsed_value);

  if (error_code != std::errc{} || ptr != last || parsed_value <= 0) {
    return false;
  }

  *result = parsed_value;
  return true;
}

}  // namespace cpp_defense
