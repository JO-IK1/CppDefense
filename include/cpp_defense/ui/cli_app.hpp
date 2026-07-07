#pragma once

#include <iosfwd>

#include "cpp_defense/core/cli_options.hpp"
#include "cpp_defense/ui/command_parser.hpp"

namespace cpp_defense {

class CliApp {
 public:
  CliApp();
  CliApp(std::istream& input, std::ostream& output,
         std::ostream& error_output);

  CliApp(const CliApp&) = delete;
  CliApp& operator=(const CliApp&) = delete;

  CliApp(CliApp&&) = delete;
  CliApp& operator=(CliApp&&) = delete;

  ~CliApp() = default;

  int Run(int argc, char* argv[]);

 private:
  void PrintHeader() const;
  void PrintUsage(std::ostream& output) const;
  void PrintParseError(const CommandParseResult& result) const;
  void PrintStartMessage(const CliOptions& options) const;

  CommandParser command_parser_;
  std::istream& input_;
  std::ostream& output_;
  std::ostream& error_output_;
};

}  // namespace cpp_defense
