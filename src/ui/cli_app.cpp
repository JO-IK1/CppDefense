#include "cpp_defense/ui/cli_app.hpp"

#include "cpp_defense/infrastructure/simple_source_parser.hpp"
#include "cpp_defense/infrastructure/source_file_repository.hpp"
#include "cpp_defense/infrastructure/file_masker.hpp"
#include "cpp_defense/infrastructure/result_file.hpp"
#include "cpp_defense/application/candidate_picker.hpp"

#include <string_view>
#include <vector>
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
          CandidatePicker candidate_picker;

          std::vector<CodeEntityInfo> all_entities;

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

            all_entities.insert(
                all_entities.end(),
                entities->begin(),
                entities->end());
          }

          output_ << "Total entities found: " << all_entities.size() << '\n';

          const CandidateSelectionMode selection_mode =
              options.functions_only()
                  ? CandidateSelectionMode::kFunctionsOnly
                  : CandidateSelectionMode::kAll;

          const auto selection = candidate_picker.Pick(
              all_entities,
              options.function_count(),
              selection_mode);

          if (!selection) {
            error_output_ << "Candidate picker error: "
                          << selection.error().FullMessage()
                          << '\n';
            break;
          }

          output_ << "\nCandidates:\n";

          for (std::size_t index = 0;
              index < selection->candidates.size();
              ++index) {
            const CodeEntityInfo& candidate =
                selection->candidates[index];

            output_ << "  [" << index << "] "
                    << '[' << EntityTypeName(candidate.type) << "] "
                    << candidate.name
                    << " | body lines: "
                    << candidate.body_line_count()
                    << '\n';
          }

          const CodeEntityInfo& selected = selection->selected();

          output_ << "\n===== SELECTED CANDIDATE =====\n";
          output_ << "Index: " << selection->selected_index << '\n';
          output_ << "Type: " << EntityTypeName(selected.type) << '\n';
          output_ << "Name: " << selected.name << '\n';
          output_ << "File: " << selected.file_path << '\n';

          output_ << "Lines: "
                  << selected.start_line
                  << '-'
                  << selected.end_line
                  << '\n';

          output_ << "Body lines: "
                  << selected.body_start_line
                  << '-'
                  << selected.body_end_line
                  << " ("
                  << selected.body_line_count()
                  << " lines)\n";

          output_ << "Offsets: ["
                  << selected.start_offset
                  << ", "
                  << selected.end_offset
                  << ")\n";

          output_ << "Body offsets: ["
                  << selected.body_start_offset
                  << ", "
                  << selected.body_end_offset
                  << ")\n";

          const auto selected_source =
              repository.ReadFile(selected.file_path);

          if (!selected_source) {
            error_output_ << selected_source.error().FullMessage()
                          << '\n';
            break;
          }

          const std::string_view selected_code(
              selected_source->data() + selected.start_offset,
              selected.end_offset - selected.start_offset);

          output_ << "\n----- SOURCE BEFORE MASK -----\n";
          output_ << selected_code << '\n';
          output_ << "------------------------------\n";

          ResultFile result_file;
          const auto result_file_create = result_file.Create(
              selected,
              project->workspace.result_path);

          if (!result_file_create) {
            error_output_ << "Result file error: "
                          << result_file_create.error().FullMessage()
                          << '\n';
            break;
          }

          output_ << "Result file created: "
                  << project->workspace.result_path
                  << '\n';

          const auto result_contents =
              result_file.Read(project->workspace.result_path);

          if (!result_contents) {
            error_output_ << "Result file error: "
                          << result_contents.error().FullMessage()
                          << '\n';
            break;
          }

          output_ << "\n----- RESULT.TXT -----\n";
          output_ << *result_contents;
          output_ << "----------------------\n";

          FileMasker file_masker;
          const auto mask_result = file_masker.Mask(selected);

          if (!mask_result) {
            error_output_ << "File masker error: "
                          << mask_result.error().FullMessage()
                          << '\n';
            break;
          }

          output_ << "Selected entity masked in cached project.\n";
          output_ << "Masked file: " << selected.file_path << '\n';

          const auto masked_source =
              repository.ReadFile(selected.file_path);

          if (!masked_source) {
            error_output_ << masked_source.error().FullMessage()
                          << '\n';
            break;
          }

          const std::string_view masked_code(
              masked_source->data() + selected.start_offset,
              selected.end_offset - selected.start_offset);

          output_ << "\n----- SOURCE AFTER MASK -----\n";
          output_ << masked_code << '\n';
          output_ << "-----------------------------\n";
          output_ << "==============================\n\n";
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
