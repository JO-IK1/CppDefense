#pragma once

#include <chrono>
#include <cstddef>
#include <expected>
#include <filesystem>
#include <optional>
#include <string>

#include "cpp_defense/application/candidate_picker.hpp"
#include "cpp_defense/application/defense_service.hpp"
#include "cpp_defense/application/defense_timer.hpp"
#include "cpp_defense/core/build_result.hpp"
#include "cpp_defense/core/code_entity_info.hpp"
#include "cpp_defense/core/defense_result.hpp"
#include "cpp_defense/core/defense_session_error.hpp"
#include "cpp_defense/core/defense_status.hpp"
#include "cpp_defense/core/workspace.hpp"
#include "cpp_defense/infrastructure/build_runner.hpp"
#include "cpp_defense/infrastructure/check_workspace.hpp"
#include "cpp_defense/infrastructure/defense_result_writer.hpp"
#include "cpp_defense/infrastructure/file_masker.hpp"
#include "cpp_defense/infrastructure/file_patcher.hpp"
#include "cpp_defense/infrastructure/result_file.hpp"
#include "cpp_defense/infrastructure/simple_source_parser.hpp"
#include "cpp_defense/infrastructure/source_file_repository.hpp"

namespace cpp_defense {

struct DefenseStartResult {
  CodeEntityInfo selected_entity;
  std::size_t source_file_count = 0;
  std::size_t entity_count = 0;
  std::size_t candidate_count = 0;
  std::filesystem::path cached_project_path;
  std::filesystem::path result_path;
  std::filesystem::path defense_result_path;
};

struct DefenseCheckResult {
  BuildResult build_result;
  DefenseStatus status = DefenseStatus::kActive;
  std::size_t attempt = 0;
};

class DefenseSession {
 public:
  explicit DefenseSession(std::filesystem::path cpp_defense_root_path);

  std::expected<DefenseStartResult, DefenseSessionError> Start(
      const std::filesystem::path& source_project_path,
      std::size_t candidate_count,
      CandidateSelectionMode mode,
      std::chrono::seconds timer_duration);

  std::expected<DefenseCheckResult, DefenseSessionError> Check();
  std::expected<DefenseResult, DefenseSessionError> Finish();

  bool ExpireIfNeeded();

  DefenseStatus status() const noexcept;
  std::chrono::seconds remaining_time() const;
  std::size_t attempts() const noexcept;
  const std::optional<CodeEntityInfo>& selected_entity() const noexcept;
  const std::optional<Workspace>& workspace() const noexcept;

 private:
  std::expected<DefenseResult, DefenseSessionError> SaveResult(DefenseStatus final_status);
  std::chrono::seconds elapsed_time() const;

  DefenseService defense_service_;
  SourceFileRepository source_file_repository_;
  SimpleSourceParser source_parser_;
  CandidatePicker candidate_picker_;
  ResultFile result_file_;
  FileMasker file_masker_;
  CheckWorkspace check_workspace_;
  FilePatcher file_patcher_;
  BuildRunner build_runner_;
  DefenseResultWriter result_writer_;
  DefenseTimer timer_;

  DefenseStatus status_ = DefenseStatus::kIdle;
  std::optional<Workspace> workspace_;
  std::optional<CodeEntityInfo> selected_entity_;
  std::optional<DefenseTimer::TimePoint> start_time_;
  std::size_t attempts_ = 0;
  std::string last_build_log_;
};

}  // namespace cpp_defense
