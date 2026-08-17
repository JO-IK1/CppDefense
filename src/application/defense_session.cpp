#include "cpp_defense/application/defense_session.hpp"

#include <string>
#include <utility>
#include <vector>

namespace cpp_defense {
namespace {

DefenseSessionError MakeError(DefenseSessionErrorType type, std::string message) {
  return DefenseSessionError(type, std::move(message));
}

class CheckWorkspaceGuard {
 public:
  CheckWorkspaceGuard(const CheckWorkspace& check_workspace, const CheckWorkspacePaths& paths)
      : check_workspace_(check_workspace), paths_(paths) {}

  ~CheckWorkspaceGuard() { check_workspace_.Cleanup(paths_); }

  CheckWorkspaceGuard(const CheckWorkspaceGuard&) = delete;
  CheckWorkspaceGuard& operator=(const CheckWorkspaceGuard&) = delete;

 private:
  const CheckWorkspace& check_workspace_;
  const CheckWorkspacePaths& paths_;
};

std::string CombinedBuildLog(const BuildResult& result) {
  std::string log;
  const auto append = [&log](std::string_view title,
                             const BuildStepResult& step) {
    if (!step.attempted || step.output.empty()) {
      return;
    }
    log += "[";
    log += title;
    log += "]\n";
    log += step.output;
    if (log.back() != '\n') {
      log.push_back('\n');
    }
  };

  append("configure", result.configure);
  append("build", result.build);
  append("tests", result.tests);
  return log;
}

}  // namespace

DefenseSession::DefenseSession(std::filesystem::path cpp_defense_root_path)
    : defense_service_(std::move(cpp_defense_root_path)) {}

std::expected<DefenseStartResult, DefenseSessionError> DefenseSession::Start(
    const std::filesystem::path& source_project_path,
    std::size_t candidate_count,
    CandidateSelectionMode mode,
    std::chrono::seconds timer_duration) {
  timer_.Stop();
  workspace_.reset();
  selected_entity_.reset();
  start_time_.reset();
  attempts_ = 0;
  last_build_log_.clear();
  status_ = DefenseStatus::kPreparing;

  const auto project = defense_service_.PrepareProject(source_project_path);
  if (!project) {
    status_ = DefenseStatus::kError;
    return std::unexpected(MakeError(
        DefenseSessionErrorType::kPreparationFailed,
        project.error().FullMessage()));
  }

  status_ = DefenseStatus::kWorkspaceReady;
  std::vector<CodeEntityInfo> entities;

  for (const std::filesystem::path& file_path : project->source_files) {
    const auto source = source_file_repository_.ReadFile(file_path);
    if (!source) {
      status_ = DefenseStatus::kError;
      return std::unexpected(MakeError(
          DefenseSessionErrorType::kSourceReadFailed,
          source.error().FullMessage()));
    }

    const auto parsed = source_parser_.Parse(*source, file_path);
    if (!parsed) {
      status_ = DefenseStatus::kError;
      return std::unexpected(MakeError(
          DefenseSessionErrorType::kParseFailed,
          parsed.error().FullMessage()));
    }

    entities.insert(entities.end(), parsed->begin(), parsed->end());
  }

  const auto selection = candidate_picker_.Pick(entities, candidate_count, mode);
  if (!selection) {
    status_ = DefenseStatus::kError;
    return std::unexpected(MakeError(
        DefenseSessionErrorType::kCandidateSelectionFailed,
        selection.error().FullMessage()));
  }

  const CodeEntityInfo selected = selection->selected();

  const auto result_create =
      result_file_.Create(selected, project->workspace.result_path);
  if (!result_create) {
    status_ = DefenseStatus::kError;
    return std::unexpected(MakeError(
        DefenseSessionErrorType::kResultFileFailed,
        result_create.error().FullMessage()));
  }

  const auto mask_result = file_masker_.Mask(selected);
  if (!mask_result) {
    status_ = DefenseStatus::kError;
    return std::unexpected(MakeError(
        DefenseSessionErrorType::kMaskingFailed,
        mask_result.error().FullMessage()));
  }

  workspace_ = project->workspace;
  selected_entity_ = selected;
  start_time_ = DefenseTimer::Clock::now();
  timer_.Start(timer_duration);
  status_ = DefenseStatus::kActive;

  return DefenseStartResult{
      .selected_entity = selected,
      .source_file_count = project->source_files.size(),
      .entity_count = entities.size(),
      .candidate_count = selection->candidates.size(),
      .cached_project_path = project->workspace.cached_project_path,
      .result_path = project->workspace.result_path,
      .defense_result_path = project->workspace.defense_result_path,
  };
}

std::expected<DefenseCheckResult, DefenseSessionError> DefenseSession::Check() {
  if (status_ == DefenseStatus::kExpired || ExpireIfNeeded()) {
    const auto saved = SaveResult(DefenseStatus::kExpired);
    if (!saved) {
      return std::unexpected(saved.error());
    }
    return std::unexpected(MakeError(
        DefenseSessionErrorType::kSessionExpired,
        "Defense time has expired"));
  }

  if (status_ != DefenseStatus::kActive || !workspace_ || !selected_entity_) {
    return std::unexpected(MakeError(
        DefenseSessionErrorType::kNoActiveSession,
        "There is no active defense session"));
  }

  status_ = DefenseStatus::kChecking;

  const auto user_solution = result_file_.Read(workspace_->result_path);
  if (!user_solution) {
    status_ = DefenseStatus::kActive;
    return std::unexpected(MakeError(
        DefenseSessionErrorType::kResultFileFailed,
        user_solution.error().FullMessage()));
  }

  const auto check_paths = check_workspace_.Prepare(*workspace_);
  if (!check_paths) {
    status_ = DefenseStatus::kActive;
    return std::unexpected(MakeError(
        DefenseSessionErrorType::kCheckWorkspaceFailed,
        check_paths.error().FullMessage()));
  }

  const CheckWorkspaceGuard cleanup_guard(check_workspace_, *check_paths);

  const auto patch_result = file_patcher_.Patch(
      *selected_entity_, workspace_->cached_project_path,
      check_paths->project_path, *user_solution);
  if (!patch_result) {
    status_ = DefenseStatus::kActive;
    return std::unexpected(MakeError(
        DefenseSessionErrorType::kPatchingFailed,
        patch_result.error().FullMessage()));
  }

  ++attempts_;
  const auto build_result = build_runner_.Run(
      check_paths->project_path, check_paths->build_path, workspace_->logs_path);
  if (!build_result) {
    status_ = DefenseStatus::kActive;
    return std::unexpected(MakeError(
        DefenseSessionErrorType::kBuildFailedToRun,
        build_result.error().FullMessage()));
  }

  last_build_log_ = CombinedBuildLog(*build_result);

  if (timer_.expired()) {
    timer_.Stop();
    status_ = DefenseStatus::kExpired;
    const auto saved = SaveResult(status_);
    if (!saved) {
      status_ = DefenseStatus::kError;
      return std::unexpected(saved.error());
    }
    return DefenseCheckResult{
        .build_result = *build_result,
        .status = status_,
        .attempt = attempts_,
    };
  }

  if (build_result->success()) {
    timer_.Stop();
    status_ = DefenseStatus::kSuccess;
    const auto saved = SaveResult(status_);
    if (!saved) {
      status_ = DefenseStatus::kError;
      return std::unexpected(saved.error());
    }
  } else {
    status_ = DefenseStatus::kActive;
  }

  return DefenseCheckResult{
      .build_result = *build_result,
      .status = status_,
      .attempt = attempts_,
  };
}

std::expected<DefenseResult, DefenseSessionError> DefenseSession::Finish() {
  if (!workspace_ || !selected_entity_) {
    return std::unexpected(MakeError(
        DefenseSessionErrorType::kNoActiveSession,
        "There is no defense session to finish"));
  }

  if (status_ == DefenseStatus::kActive || status_ == DefenseStatus::kChecking) {
    timer_.Stop();
    status_ = DefenseStatus::kFailed;
  } else if (status_ == DefenseStatus::kExpired) {
    timer_.Stop();
  }

  return SaveResult(status_);
}

std::expected<DefenseResult, DefenseSessionError> DefenseSession::SaveResult(
    DefenseStatus final_status) {
  if (!workspace_ || !selected_entity_) {
    return std::unexpected(MakeError(
        DefenseSessionErrorType::kNoActiveSession,
        "Cannot save a result without an initialized session"));
  }

  const DefenseResult result{
      .selected_entity = *selected_entity_,
      .status = final_status,
      .attempts = attempts_,
      .elapsed_time = elapsed_time(),
      .last_build_log = last_build_log_,
  };

  const auto write_result =
      result_writer_.Write(result, workspace_->defense_result_path);
  if (!write_result) {
    return std::unexpected(MakeError(
        DefenseSessionErrorType::kResultWriteFailed,
        write_result.error().FullMessage()));
  }

  return result;
}

bool DefenseSession::ExpireIfNeeded() {
  if (status_ == DefenseStatus::kActive && timer_.expired()) {
    timer_.Stop();
    status_ = DefenseStatus::kExpired;
    return true;
  }
  return false;
}

DefenseStatus DefenseSession::status() const noexcept { return status_; }

std::chrono::seconds DefenseSession::remaining_time() const {
  return timer_.remaining();
}

std::size_t DefenseSession::attempts() const noexcept { return attempts_; }

const std::optional<CodeEntityInfo>&
DefenseSession::selected_entity() const noexcept {
  return selected_entity_;
}

const std::optional<Workspace>& DefenseSession::workspace() const noexcept {
  return workspace_;
}

std::chrono::seconds DefenseSession::elapsed_time() const {
  if (!start_time_) {
    return std::chrono::seconds::zero();
  }
  return std::chrono::duration_cast<std::chrono::seconds>(
      DefenseTimer::Clock::now() - *start_time_);
}

}  // namespace cpp_defense
