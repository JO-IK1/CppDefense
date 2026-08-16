#include "cpp_defense/infrastructure/file_masker.hpp"

#include <cerrno>
#include <cstddef>
#include <fstream>
#include <iterator>
#include <string>
#include <system_error>

namespace cpp_defense {
namespace {

bool IsValidBodyRange(const CodeEntityInfo& entity, std::size_t source_size) {
  return entity.body_start_offset < entity.body_end_offset &&
         entity.body_end_offset <= source_size;
}

bool HasExpectedBodyBraces(const std::string& source, const CodeEntityInfo& entity) {
  return source[entity.body_start_offset] == '{' &&
         source[entity.body_end_offset - 1] == '}';
}

}  // namespace

std::expected<void, FileMaskerError> FileMasker::Mask(
    const CodeEntityInfo& entity) const {
  errno = 0;

  std::ifstream input(entity.file_path, std::ios::binary);

  if (!input.is_open()) {
    return std::unexpected(FileMaskerOpenFailed(
        entity.file_path,
        std::error_code(errno, std::generic_category())));
  }

  std::string contents{
      std::istreambuf_iterator<char>(input),
      std::istreambuf_iterator<char>()};

  if (input.bad()) {
    return std::unexpected(FileMaskerReadFailed(
        entity.file_path,
        std::error_code(errno, std::generic_category())));
  }

  if (!IsValidBodyRange(entity, contents.size())) {
    return std::unexpected(FileMaskerInvalidEntityRange(entity.file_path));
  }

  if (!HasExpectedBodyBraces(contents, entity)) {
    return std::unexpected(FileMaskerBodyBoundaryMismatch(entity.file_path));
  }

  input.close();

  for (std::size_t offset = entity.body_start_offset + 1;
       offset < entity.body_end_offset - 1;
       ++offset) {
    if (contents[offset] != '\n' && contents[offset] != '\r') {
      contents[offset] = ' ';
    }
  }

  errno = 0;

  std::ofstream output(entity.file_path, std::ios::binary | std::ios::trunc);

  if (!output.is_open()) {
    return std::unexpected(FileMaskerWriteFailed(
        entity.file_path,
        std::error_code(errno, std::generic_category())));
  }

  output.write(
      contents.data(),
      static_cast<std::streamsize>(contents.size()));

  if (!output) {
    return std::unexpected(FileMaskerWriteFailed(
        entity.file_path,
        std::error_code(errno, std::generic_category())));
  }

  return {};
}

}  // namespace cpp_defense
