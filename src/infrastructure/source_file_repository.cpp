#include "cpp_defense/infrastructure/source_file_repository.hpp"

#include <cerrno>
#include <fstream>
#include <iterator>
#include <string>
#include <system_error>

namespace cpp_defense {

std::expected<std::string, SourceFileError> SourceFileRepository::ReadFile(
    const std::filesystem::path& file_path) const {
  errno = 0;

  std::ifstream input(file_path, std::ios::binary);

  if (!input.is_open()) {
    return std::unexpected(SourceFileOpenFailed(
        file_path, std::error_code(errno, std::generic_category())));
  }

  std::string contents{
      std::istreambuf_iterator<char>(input),
      std::istreambuf_iterator<char>()};

  if (input.bad()) {
    return std::unexpected(SourceFileReadFailed(
        file_path, std::error_code(errno, std::generic_category())));
  }

  return contents;
}

}  // namespace cpp_defense
