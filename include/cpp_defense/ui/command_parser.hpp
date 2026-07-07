#pragma once

#include <string>
#include <string_view>

#include "cpp_defense/core/cli_options.hpp"

namespace cpp_defense {

enum class CommandParseStatus {
  kSuccess,
  kHelpRequested,
  kMissingProjectPath,
  kProjectPathDoesNotExist,
  kProjectPathIsNotDirectory,
  kMissingOptionValue,
  kInvalidFunctionCount,
  kInvalidTimer,
  kUnknownArgument,
};

class CommandParseResult {
 public:
  static CommandParseResult Success(CliOptions options);
  static CommandParseResult HelpRequested();
  static CommandParseResult Error(CommandParseStatus status, std::string message);

  bool ok() const noexcept;
  bool help_requested() const noexcept;

  CommandParseStatus status() const noexcept { return status_; }
  const CliOptions& options() const noexcept { return options_; }
  const std::string& message() const noexcept { return message_; }

 private:
  CommandParseResult(CommandParseStatus status, CliOptions options,
                     std::string message);

  CommandParseStatus status_ = CommandParseStatus::kSuccess;
  CliOptions options_;
  std::string message_;
};

class CommandParser {
 public:
  CommandParser() = default;

  CommandParseResult Parse(int argc, char* argv[]) const;

  static std::string_view ProgramName() noexcept;
  static std::string UsageText();

 private:
  static bool IsHelpArgument(std::string_view argument) noexcept;
  static bool IsFunctionCountOption(std::string_view argument) noexcept;
  static bool IsTimerOption(std::string_view argument) noexcept;
  static bool TryParsePositiveInt(std::string_view value, int* result) noexcept;
};

}  // namespace cpp_defense
