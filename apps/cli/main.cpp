#include <filesystem>
#include <iostream>
#include <string>
#include <system_error>

#include "cpp_defense/ui/cli_app.hpp"

namespace {

std::filesystem::path ResolveCppDefenseRoot(
    const std::filesystem::path& executable_argument) {
  std::error_code error_code;
  std::filesystem::path executable_path =
      std::filesystem::absolute(executable_argument, error_code);
  if (error_code) {
    return ".";
  }

  const std::filesystem::path canonical_path =
      std::filesystem::canonical(executable_path, error_code);
  if (!error_code) {
    executable_path = canonical_path;
  }

  std::filesystem::path executable_directory = executable_path.parent_path();
  const std::string directory_name =
      executable_directory.filename().string();
  if (directory_name == "Debug" || directory_name == "Release" ||
      directory_name == "RelWithDebInfo" || directory_name == "MinSizeRel") {
    executable_directory = executable_directory.parent_path();
  }

  if (executable_directory.filename() == "build" ||
      executable_directory.filename() == "bin") {
    return executable_directory.parent_path();
  }

  return executable_directory;
}

}  // namespace

int main(int argc, char* argv[]) {
  cpp_defense::CliApp app(std::cin, std::cout, std::cerr,
                          ResolveCppDefenseRoot(argv[0]));
  return app.Run(argc, argv);
}
