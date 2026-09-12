#include "workspace.hpp"
#include "cpp_defense/infrastructure/session_id.hpp"
#include <algorithm>
#include <array>
#include <fstream>
#ifdef _WIN32
#define NOMINMAX
#include <windows.h>
#else
#include <fcntl.h>
#include <sys/file.h>
#include <unistd.h>
#endif

namespace cpp_defense::worker {
fs::path Resolve(const fs::path& root, std::string_view relative, bool existing) {
  Require(RelativePath(relative), "INVALID_PATH", "Unsafe relative path");
  auto result = root;
  for (const auto& part : fs::path(relative)) {
    result /= part;
    std::error_code ec;
    const auto status = fs::symlink_status(result, ec);
    if (ec == std::errc::no_such_file_or_directory && !existing) continue;
    Require(!ec, "INVALID_PATH", "Path does not exist or cannot be inspected");
    Require(!fs::is_symlink(status), "INVALID_PATH", "Symbolic links are forbidden");
    if (fs::is_regular_file(status)) {
      Require(fs::hard_link_count(result) == 1, "INVALID_PATH", "Hard links are forbidden");
    }
    Require(fs::is_directory(status) || fs::is_regular_file(status) ||
            (!existing && !fs::exists(status)), "INVALID_PATH", "Special file is forbidden");
  }
  return result;
}

std::vector<fs::path> ValidateTree(const fs::path& root) {
  Require(fs::is_directory(root), "INVALID_PROJECT", "Project root must be a directory");
  std::vector<fs::path> files;
  std::uintmax_t total = 0;
  std::size_t entries = 0;
  for (fs::recursive_directory_iterator it(root), end; it != end; ++it) {
    Require(++entries <= 20000 && it.depth() < 32, "PROJECT_LIMIT", "Project entry/depth limit exceeded");
    const auto relative = it->path().lexically_relative(root).generic_string();
    const auto path = Resolve(root, relative);
    if (it->is_regular_file()) {
      const auto size = it->file_size();
      total += size;
      Require(size <= 16*1024*1024 && total <= 512*1024*1024,
              "PROJECT_LIMIT", "Project size limit exceeded");
      files.push_back(path);
    }
  }
  std::sort(files.begin(), files.end());
  return files;
}

std::string Read(const fs::path& path) {
  Require(fs::is_regular_file(path) && fs::file_size(path) <= 16*1024*1024,
          "INVALID_PROJECT", "Expected a regular file of at most 16 MiB");
  std::ifstream in(path, std::ios::binary);
  if (!in) throw Error("IO_ERROR", "Cannot read workspace file", "internal", true);
  std::string result;
  std::array<char, 8192> buffer{};
  while (in.read(buffer.data(), buffer.size()) || in.gcount()) {
    result.append(buffer.data(), static_cast<std::size_t>(in.gcount()));
    Require(result.size() <= 16*1024*1024, "PROJECT_LIMIT", "File grew beyond the limit");
  }
  if (in.bad()) throw Error("IO_ERROR", "Workspace read failed", "internal", true);
  return result;
}

void Write(const fs::path& path, std::string_view bytes) {
  std::ofstream out(path, std::ios::binary | std::ios::trunc);
  out.write(bytes.data(), static_cast<std::streamsize>(bytes.size()));
  out.close();
  if (!out) throw Error("IO_ERROR", "Workspace write failed", "internal", true);
}

SessionLock::SessionLock(const fs::path& session) {
  const auto path = Resolve(session, ".worker.lock", false);
#ifdef _WIN32
  handle_ = CreateFileW(path.c_str(), GENERIC_READ | GENERIC_WRITE, 0, nullptr,
                        OPEN_ALWAYS, FILE_ATTRIBUTE_NORMAL, nullptr);
  if (handle_ == INVALID_HANDLE_VALUE) {
    handle_ = nullptr;
    throw Error("SESSION_BUSY", "Session is busy", "conflict", true);
  }
#else
  handle_ = open(path.c_str(), O_RDWR | O_CREAT | O_NOFOLLOW, 0600);
  if (handle_ < 0) throw Error("IO_ERROR", "Cannot lock session", "internal", true);
  if (flock(handle_, LOCK_EX | LOCK_NB) != 0) {
    close(handle_);
    handle_ = -1;
    throw Error("SESSION_BUSY", "Session is busy", "conflict", true);
  }
#endif
}
SessionLock::~SessionLock() {
#ifdef _WIN32
  if (handle_) CloseHandle(handle_);
#else
  if (handle_ >= 0) close(handle_);
#endif
}

TemporaryTree::TemporaryTree(fs::path path) : path_(std::move(path)) {
  Require(fs::create_directory(path_), "OUTPUT_EXISTS", "Temporary directory already exists");
}
TemporaryTree::~TemporaryTree() {
  if (!committed_) {
    std::error_code ignored;
    fs::remove_all(path_, ignored);
  }
}
void TemporaryTree::Commit(const fs::path& destination) {
  Require(!fs::exists(destination), "OUTPUT_EXISTS", "Output directory already exists");
  fs::rename(path_, destination);
  committed_ = true;
}
void SaveState(const fs::path& session, const Json& state) {
  const auto destination = Resolve(session, "defense-state.json", false);
  Require(!fs::exists(destination), "SESSION_CONFLICT", "Preparation state already exists");
  TemporaryTree temporary(Resolve(session, ".state-" + NewSessionId(), false));
  Write(temporary.path() / "state.json", state.dump());
  fs::rename(temporary.path() / "state.json", destination);
}
}  // namespace cpp_defense::worker
