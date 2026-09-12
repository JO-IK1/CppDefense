#pragma once
#include "protocol.hpp"
#include <filesystem>
#include <string>
#include <vector>

namespace cpp_defense::worker {
namespace fs = std::filesystem;
fs::path Resolve(const fs::path& root, std::string_view relative, bool existing = true);
std::vector<fs::path> ValidateTree(const fs::path& root);
std::string Read(const fs::path& path);
void Write(const fs::path& path, std::string_view bytes);
void SaveState(const fs::path& session, const Json& state);
class SessionLock {
 public:
  explicit SessionLock(const fs::path& session);
  ~SessionLock();
  SessionLock(const SessionLock&) = delete;
  SessionLock& operator=(const SessionLock&) = delete;
 private:
#ifdef _WIN32
  void* handle_ = nullptr;
#else
  int handle_ = -1;
#endif
};
class TemporaryTree {
 public:
  explicit TemporaryTree(fs::path path);
  ~TemporaryTree();
  void Commit(const fs::path& destination);
  const fs::path& path() const { return path_; }
 private:
  fs::path path_;
  bool committed_ = false;
};
}  // namespace cpp_defense::worker
