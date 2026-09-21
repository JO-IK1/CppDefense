#include "process_runner.hpp"

#include <algorithm>
#include <chrono>
#include <string>
#include <thread>
#include <utility>
#include <vector>

#ifdef _WIN32
#include <windows.h>
#else
#include <cerrno>
#include <csignal>
#include <fcntl.h>
#include <sys/wait.h>
#include <unistd.h>
#endif

namespace cpp_defense::process_runner {
namespace {

BuildRunnerError StartError(std::string message,
                            const std::filesystem::path& log_path) {
  return BuildRunnerError(BuildRunnerErrorType::kFailedToRunCommand,
                          std::move(message), log_path);
}

#ifdef _WIN32

class Handle {
 public:
  explicit Handle(HANDLE value = nullptr) : value_(value) {}
  ~Handle() {
    if (value_ != nullptr && value_ != INVALID_HANDLE_VALUE) {
      CloseHandle(value_);
    }
  }

  Handle(const Handle&) = delete;
  Handle& operator=(const Handle&) = delete;

  HANDLE get() const { return value_; }
  explicit operator bool() const {
    return value_ != nullptr && value_ != INVALID_HANDLE_VALUE;
  }

 private:
  HANDLE value_;
};

std::wstring QuoteWindowsArgument(const std::wstring& argument) {
  if (argument.empty()) {
    return L"\"\"";
  }

  if (argument.find_first_of(L" \t\n\v\"") == std::wstring::npos) {
    return argument;
  }

  std::wstring quoted = L"\"";
  std::size_t backslashes = 0;
  for (const wchar_t character : argument) {
    if (character == L'\\') {
      ++backslashes;
      continue;
    }
    if (character == L'\"') {
      quoted.append(backslashes * 2 + 1, L'\\');
      quoted.push_back(character);
      backslashes = 0;
      continue;
    }
    quoted.append(backslashes, L'\\');
    backslashes = 0;
    quoted.push_back(character);
  }
  quoted.append(backslashes * 2, L'\\');
  quoted.push_back(L'\"');
  return quoted;
}

std::wstring BuildCommandLine(const std::string& executable,
                              const std::vector<std::string>& arguments) {
  std::wstring command = QuoteWindowsArgument(
      std::filesystem::path(executable).wstring());
  for (const std::string& argument : arguments) {
    command.push_back(L' ');
    command += QuoteWindowsArgument(std::filesystem::path(argument).wstring());
  }
  return command;
}

std::expected<Result, BuildRunnerError> RunPlatform(
    const std::string& executable,
    const std::vector<std::string>& arguments,
    const std::filesystem::path& log_path,
    std::chrono::milliseconds timeout) {
  SECURITY_ATTRIBUTES attributes{};
  attributes.nLength = sizeof(attributes);
  attributes.bInheritHandle = TRUE;

  Handle log(CreateFileW(log_path.c_str(), GENERIC_WRITE,
                         FILE_SHARE_READ | FILE_SHARE_WRITE, &attributes,
                         CREATE_ALWAYS, FILE_ATTRIBUTE_NORMAL, nullptr));
  if (!log) {
    return std::unexpected(StartError("Failed to open process log", log_path));
  }

  STARTUPINFOW startup{};
  startup.cb = sizeof(startup);
  startup.dwFlags = STARTF_USESTDHANDLES;
  startup.hStdInput = GetStdHandle(STD_INPUT_HANDLE);
  startup.hStdOutput = log.get();
  startup.hStdError = log.get();

  PROCESS_INFORMATION process_info{};
  std::wstring command_line = BuildCommandLine(executable, arguments);
  if (!CreateProcessW(nullptr, command_line.data(), nullptr, nullptr, TRUE,
                      CREATE_SUSPENDED | CREATE_NO_WINDOW, nullptr, nullptr,
                      &startup, &process_info)) {
    return std::unexpected(StartError("Failed to start process", log_path));
  }

  Handle process(process_info.hProcess);
  Handle thread(process_info.hThread);
  Handle job(CreateJobObjectW(nullptr, nullptr));
  bool assigned_to_job = false;
  if (job) {
    JOBOBJECT_EXTENDED_LIMIT_INFORMATION limits{};
    limits.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE;
    SetInformationJobObject(job.get(), JobObjectExtendedLimitInformation,
                            &limits, sizeof(limits));
    assigned_to_job = AssignProcessToJobObject(job.get(), process.get()) != 0;
  }
  if (ResumeThread(thread.get()) == static_cast<DWORD>(-1)) {
    TerminateProcess(process.get(), 125);
    WaitForSingleObject(process.get(), INFINITE);
    return std::unexpected(StartError("Failed to resume process", log_path));
  }

  const auto bounded_timeout = std::clamp<long long>(
      timeout.count(), 0, static_cast<long long>(INFINITE - 1));
  const DWORD wait_result = WaitForSingleObject(
      process.get(), static_cast<DWORD>(bounded_timeout));

  if (wait_result == WAIT_TIMEOUT) {
    if (assigned_to_job) {
      TerminateJobObject(job.get(), 124);
    } else {
      TerminateProcess(process.get(), 124);
    }
    WaitForSingleObject(process.get(), INFINITE);
    return Result{.exit_code = 124, .timed_out = true};
  }
  if (wait_result != WAIT_OBJECT_0) {
    if (assigned_to_job) {
      TerminateJobObject(job.get(), 125);
    } else {
      TerminateProcess(process.get(), 125);
    }
    WaitForSingleObject(process.get(), INFINITE);
    return std::unexpected(StartError("Failed while waiting for process", log_path));
  }

  DWORD exit_code = 0;
  if (!GetExitCodeProcess(process.get(), &exit_code)) {
    return std::unexpected(StartError("Failed to read process exit code", log_path));
  }
  return Result{.exit_code = static_cast<int>(exit_code)};
}

#else

std::expected<Result, BuildRunnerError> RunPlatform(
    const std::string& executable,
    const std::vector<std::string>& arguments,
    const std::filesystem::path& log_path,
    std::chrono::milliseconds timeout) {
  const int log_fd = open(log_path.c_str(), O_WRONLY | O_CREAT | O_TRUNC, 0644);
  if (log_fd == -1) {
    return std::unexpected(StartError("Failed to open process log", log_path));
  }

  const pid_t process_id = fork();
  if (process_id == -1) {
    close(log_fd);
    return std::unexpected(StartError("Failed to fork process", log_path));
  }

  if (process_id == 0) {
    setpgid(0, 0);
    if (dup2(log_fd, STDOUT_FILENO) == -1 ||
        dup2(log_fd, STDERR_FILENO) == -1) {
      _exit(126);
    }
    close(log_fd);

    std::vector<char*> argv;
    argv.reserve(arguments.size() + 2);
    argv.push_back(const_cast<char*>(executable.c_str()));
    for (const std::string& argument : arguments) {
      argv.push_back(const_cast<char*>(argument.c_str()));
    }
    argv.push_back(nullptr);
    execvp(executable.c_str(), argv.data());
    _exit(errno == ENOENT ? 127 : 126);
  }

  close(log_fd);
  setpgid(process_id, process_id);
  const auto deadline = std::chrono::steady_clock::now() + timeout;
  int status = 0;

  while (true) {
    const pid_t wait_result = waitpid(process_id, &status, WNOHANG);
    if (wait_result == process_id) {
      if (WIFEXITED(status)) {
        return Result{.exit_code = WEXITSTATUS(status)};
      }
      if (WIFSIGNALED(status)) {
        return Result{.exit_code = 128 + WTERMSIG(status)};
      }
    } else if (wait_result == -1 && errno != EINTR) {
      return std::unexpected(StartError("Failed while waiting for process", log_path));
    }

    if (std::chrono::steady_clock::now() >= deadline) {
      kill(-process_id, SIGKILL);
      kill(process_id, SIGKILL);
      while (waitpid(process_id, &status, 0) == -1 && errno == EINTR) {
      }
      return Result{.exit_code = 124, .timed_out = true};
    }
    std::this_thread::sleep_for(std::chrono::milliseconds(10));
  }
}

#endif

}  // namespace

std::expected<Result, BuildRunnerError> Run(
    const std::string& executable,
    const std::vector<std::string>& arguments,
    const std::filesystem::path& log_path,
    std::chrono::milliseconds timeout) {
  if (timeout <= std::chrono::milliseconds::zero()) {
    return Result{.exit_code = 124, .timed_out = true};
  }
  return RunPlatform(executable, arguments, log_path, timeout);
}

}  // namespace cpp_defense::process_runner
