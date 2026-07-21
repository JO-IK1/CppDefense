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

struct CommandParts {
  std::string_view name;
  std::string_view value;
};

std::string_view Trim(std::string_view value) noexcept {
  constexpr std::string_view kWhitespace = " \t\n\r\f\v";
  const std::size_t first = value.find_first_not_of(kWhitespace);
  if (first == std::string_view::npos) {
    return {};
  }

  const std::size_t last = value.find_last_not_of(kWhitespace);
  return value.substr(first, last - first + 1);
}

CommandParts SplitCommand(std::string_view input) noexcept {
  input = Trim(input);
  const std::size_t separator = input.find_first_of(" \t");
  if (separator == std::string_view::npos) {
    return {input, {}};
  }

  return {input.substr(0, separator), Trim(input.substr(separator + 1))};
}

std::string ValidateProjectPath(const std::filesystem::path& path) {
  std::error_code error_code;
  const bool exists = std::filesystem::exists(path, error_code);
  if (error_code || !exists) {
    return "Project path does not exist: " + path.string();
  }

  const bool is_directory = std::filesystem::is_directory(path, error_code);
  if (error_code || !is_directory) {
    return "Project path is not a directory: " + path.string();
  }

  return {};
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

InteractiveParseResult InteractiveParseResult::Success(InteractiveCommandType type) {
  return InteractiveParseResult(true, type, "");
}

InteractiveParseResult InteractiveParseResult::Error(std::string message) {
  return InteractiveParseResult(false, InteractiveCommandType::kEmpty, std::move(message));
}

InteractiveParseResult::InteractiveParseResult(bool ok,
                                               InteractiveCommandType type,
                                               std::string message)
    : ok_(ok), type_(type), message_(std::move(message)) {}

CommandParseResult CommandParser::Parse(int argc, char* argv[]) const {
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

    if (argument == "-p" || argument == "--path") {
      if (index + 1 >= argc) {
        return CommandParseResult::Error(
            CommandParseStatus::kMissingOptionValue,
            "Project path option requires a value.");
      }

      if (project_path_set) {
        return CommandParseResult::Error(
            CommandParseStatus::kUnknownArgument,
            "Only one project path can be provided.");
      }

      options.set_project_path(std::filesystem::path(argv[++index]));
      project_path_set = true;
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

  if (project_path_set) {
    const std::string path_error = ValidateProjectPath(options.project_path());
    if (!path_error.empty()) {
      return CommandParseResult::Error(
          path_error.starts_with("Project path does not exist:")
              ? CommandParseStatus::kProjectPathDoesNotExist
              : CommandParseStatus::kProjectPathIsNotDirectory,
          path_error);
    }
  }

  return CommandParseResult::Success(std::move(options));
}

InteractiveParseResult CommandParser::ParseInteractive(
    std::string_view input, CliOptions& options) const {
  const CommandParts command = SplitCommand(input);

  if (command.name.empty()) {
    return InteractiveParseResult::Success(InteractiveCommandType::kEmpty);
  }

  const auto require_no_value = [&command](InteractiveCommandType type,
                                            std::string_view usage) {
    if (!command.value.empty()) {
      return InteractiveParseResult::Error("Usage: " + std::string(usage));
    }
    return InteractiveParseResult::Success(type);
  };

  if (IsHelpArgument(command.name)) {
    return require_no_value(InteractiveCommandType::kHelp, "-h");
  }

  if (command.name == "-e" || command.name == "--exit") {
    return require_no_value(InteractiveCommandType::kExit, "-e");
  }

  if (command.name == "-s" || command.name == "--start") {
    return require_no_value(InteractiveCommandType::kStart, "-s");
  }

  if (command.name == "-c" || command.name == "--check") {
    return require_no_value(InteractiveCommandType::kCheck, "-c");
  }

  if (command.name == "-p" || command.name == "--path") {
    if (command.value.empty()) {
      return InteractiveParseResult::Error("Usage: -p <project_path>");
    }

    const std::filesystem::path path(command.value);
    const std::string path_error = ValidateProjectPath(path);
    if (!path_error.empty()) {
      return InteractiveParseResult::Error(path_error);
    }

    options.set_project_path(path);
    return InteractiveParseResult::Success(
        InteractiveCommandType::kSetPath);
  }

  if (IsFunctionCountOption(command.name)) {
    int function_count = 0;
    if (!TryParsePositiveInt(command.value, &function_count) ||
        function_count > kMaxFunctionCount) {
      return InteractiveParseResult::Error("Function count must be an integer from 1 to 50.");
    }

    options.set_function_count(function_count);
    return InteractiveParseResult::Success(InteractiveCommandType::kSetFunctionCount);
  }

  if (IsTimerOption(command.name)) {
    int timer_minutes = 0;
    if (!TryParsePositiveInt(command.value, &timer_minutes) ||
        timer_minutes > kMaxTimerMinutes) {
      return InteractiveParseResult::Error("Timer must be an integer from 1 to 180 minutes.");
    }

    options.set_timer_minutes(timer_minutes);
    return InteractiveParseResult::Success(InteractiveCommandType::kSetTimer);
  }

  if (command.name == "--functions-only") {
    const InteractiveParseResult result = require_no_value(
        InteractiveCommandType::kFunctionsOnly, "--functions-only");
    if (result.ok()) {
      options.set_functions_only(true);
    }
    return result;
  }

  if (command.name == "--all") {
    const InteractiveParseResult result =
        require_no_value(InteractiveCommandType::kAll, "--all");
    if (result.ok()) {
      options.set_functions_only(false);
    }
    return result;
  }

  return InteractiveParseResult::Error(
      "Unknown command: " + std::string(command.name));
}

std::string_view CommandParser::ProgramName() noexcept { return "cpp-defense"; }

std::string CommandParser::UsageText() {
  return "Usage: cpp-defense [project_path] [options]\n"
         "\n"
         "Options:\n"
         "  -h, --help                 Show this help message.\n"
         "  -p, --path <directory>     Select a project directory\n"
         "  -n, --functions <count>    Number of candidate functions.\n"
         "  -t, --timer <minutes>      Defense timer in minutes.\n"
         "      --functions-only       Pick only functions. Default behavior.\n"
         "      --all                  Allow all supported code fragments.\n"
         "\n"
         "Interactive commands:\n"
         "  -h, --help                 Show this help message.\n"
         "  -p, --path <directory>     Select a project directory.\n"
         "  -n, --functions <count>    Change candidate function count.\n"
         "  -t, --timer <minutes>      Change defense timer.\n"
         "      --functions-only       Pick only functions.\n"
         "      --all                  Allow all supported code fragments.\n"
         "  -s, --start                Pick a function and remove its body.\n"
         "  -c, --check                Check the restored function.\n"
         "  -e, --exit                 Close the application.\n";
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
