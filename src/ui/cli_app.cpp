#include "cpp_defense/ui/cli_app.hpp"

#include "cpp_defense/infrastructure/simple_source_parser.hpp"
#include "cpp_defense/infrastructure/source_file_repository.hpp"

#include <filesystem>
#include <iostream>
#include <ostream>
#include <string>
#include <utility>

namespace cpp_defense {
namespace {

constexpr int kSuccessExitCode = 0;
constexpr int kErrorExitCode = 1;

}  // namespace

CliApp::CliApp() : CliApp(std::cin, std::cout, std::cerr, ".") {}

CliApp::CliApp(std::istream& input, std::ostream& output,
               std::ostream& error_output)
    : CliApp(input, output, error_output, ".") {}

CliApp::CliApp(std::istream& input, std::ostream& output,
               std::ostream& error_output,
               std::filesystem::path cpp_defense_root_path)
    : defense_service_(std::move(cpp_defense_root_path)),
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
      case InteractiveCommandType::kExit:
        return kSuccessExitCode;
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

        const auto project =
            defense_service_.PrepareProject(options.project_path());

        if (project) {
          output_ << "Workspace ready: " << project->workspace.cached_project_path << '\n';

          output_ << "Source files found: " << project->source_files.size() << '\n';
          
          // DEBUG ONLY
          for (const auto& file_path : project->source_files) {
            output_ << "  - " << file_path << '\n';
          }

          SourceFileRepository repository;
          SimpleSourceParser parser;

          output_ << "Entities found:\n";

          const auto EntityTypeName = [](CodeEntityType type) -> std::string_view {
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
          };

          std::size_t entity_count = 0;

          for (const auto& file_path : project->source_files) {
            const auto source = repository.ReadFile(file_path);
            if (!source) {
              error_output_ << source.error().FullMessage() << '\n';
              continue;
            }

            const auto entities = parser.Parse(*source, file_path);

            if (!entities) {
              error_output_ << entities.error().FullMessage() << '\n';
              continue;
            }

            for (const auto& entity : *entities) {
              output_ << "  - [" << EntityTypeName(entity.type) << "] "
                      << entity.name << " [lines " << entity.start_line
                      << '-' << entity.end_line << "]\n";

              ++entity_count;
            }
          }

          output_ << "Total entities found: " << entity_count << '\n';
          // END DEBUG ONLY
        } else {
          error_output_ << "Error: " << project.error().FullMessage() << '\n';
        }
        break;
      }
      case InteractiveCommandType::kCheck:
        output_ << "Check function...\n";
        break;
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
