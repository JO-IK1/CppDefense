#include "cpp_defense/infrastructure/result_file.hpp"

#include <cerrno>
#include <fstream>
#include <iterator>
#include <string>
#include <string_view>
#include <system_error>

namespace cpp_defense {
namespace {

bool IsValidEntityRange(const CodeEntityInfo& entity,
                        std::size_t source_size) {
  return entity.start_offset <= entity.body_start_offset &&
         entity.body_start_offset < entity.body_end_offset &&
         entity.body_end_offset <= entity.end_offset &&
         entity.end_offset <= source_size;
}

bool HasExpectedBodyBraces(const std::string& source,
                           const CodeEntityInfo& entity) {
  return source[entity.body_start_offset] == '{' &&
         source[entity.body_end_offset - 1] == '}';
}

std::string BuildResultTemplate(const std::string& source,
                                const CodeEntityInfo& entity) {
  const std::string_view signature(
      source.data() + entity.start_offset,
      entity.body_start_offset - entity.start_offset);

  const std::string_view suffix(
      source.data() + entity.body_end_offset,
      entity.end_offset - entity.body_end_offset);

  std::string result;
  result.reserve(signature.size() + suffix.size() + 6);

  result.append(signature);
  result.append("{\n\n}");
  result.append(suffix);

  if (result.empty() || result.back() != '\n') {
    result.push_back('\n');
  }

  return result;
}

}  // namespace

std::expected<void, ResultFileError> ResultFile::Create(
    const CodeEntityInfo& entity,
    const std::filesystem::path& result_path) const {
  errno = 0;

  std::ifstream input(entity.file_path, std::ios::binary);

  if (!input.is_open()) {
    return std::unexpected(ResultSourceOpenFailed(
        entity.file_path,
        std::error_code(errno, std::generic_category())));
  }

  std::string source{
      std::istreambuf_iterator<char>(input),
      std::istreambuf_iterator<char>()};

  if (input.bad()) {
    return std::unexpected(ResultSourceReadFailed(
        entity.file_path,
        std::error_code(errno, std::generic_category())));
  }

  if (!IsValidEntityRange(entity, source.size())) {
    return std::unexpected(
        ResultInvalidEntityRange(entity.file_path));
  }

  if (!HasExpectedBodyBraces(source, entity)) {
    return std::unexpected(
        ResultBodyBoundaryMismatch(entity.file_path));
  }

  const std::string result = BuildResultTemplate(source, entity);

  errno = 0;

  std::ofstream output(result_path, std::ios::binary | std::ios::trunc);

  if (!output.is_open()) {
    return std::unexpected(ResultFileOpenFailed(
        result_path,
        std::error_code(errno, std::generic_category())));
  }

  output.write(
      result.data(),
      static_cast<std::streamsize>(result.size()));

  if (!output) {
    return std::unexpected(ResultFileWriteFailed(
        result_path,
        std::error_code(errno, std::generic_category())));
  }

  return {};
}

std::expected<std::string, ResultFileError> ResultFile::Read(
    const std::filesystem::path& result_path) const {
  errno = 0;

  std::ifstream input(result_path, std::ios::binary);

  if (!input.is_open()) {
    return std::unexpected(ResultFileOpenFailed(
        result_path,
        std::error_code(errno, std::generic_category())));
  }

  std::string contents{
      std::istreambuf_iterator<char>(input),
      std::istreambuf_iterator<char>()};

  if (input.bad()) {
    return std::unexpected(ResultFileReadFailed(
        result_path,
        std::error_code(errno, std::generic_category())));
  }

  return contents;
}

}  // namespace cpp_defense
