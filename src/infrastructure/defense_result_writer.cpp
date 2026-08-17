#include "cpp_defense/infrastructure/defense_result_writer.hpp"

#include <fstream>
#include <iomanip>
#include <string_view>

namespace cpp_defense {
namespace {

std::string_view StatusName(DefenseStatus status) {
  switch (status) {
    case DefenseStatus::kSuccess:
      return "success";
    case DefenseStatus::kExpired:
      return "expired";
    case DefenseStatus::kFailed:
      return "failed";
    case DefenseStatus::kError:
      return "error";
    default:
      return "unfinished";
  }
}

}  // namespace

std::expected<void, DefenseResultError> DefenseResultWriter::Write(
    const DefenseResult& result,
    const std::filesystem::path& output_path) const {
  std::ofstream output(output_path, std::ios::binary | std::ios::trunc);
  if (!output.is_open()) {
    return std::unexpected(DefenseResultError(
        "Failed to open defense result file", output_path));
  }

  const auto total_seconds = result.elapsed_time.count();
  const auto minutes = total_seconds / 60;
  const auto seconds = total_seconds % 60;

  output << "CppDefense Result\n"
         << "Entity: " << result.selected_entity.name << '\n'
         << "File: " << result.selected_entity.file_path << '\n'
         << "Attempts: " << result.attempts << '\n'
         << "Elapsed: " << std::setfill('0') << std::setw(2) << minutes
         << ':' << std::setw(2) << seconds << std::setfill(' ') << '\n'
         << "Result: " << StatusName(result.status) << '\n';

  if (!result.last_build_log.empty()) {
    output << "\nLast build/test log:\n" << result.last_build_log;
    if (result.last_build_log.back() != '\n') {
      output << '\n';
    }
  }

  if (!output) {
    return std::unexpected(DefenseResultError(
        "Failed to write defense result file", output_path));
  }

  return {};
}

}  // namespace cpp_defense
