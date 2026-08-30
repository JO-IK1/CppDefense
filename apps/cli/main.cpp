#include <iostream>

#include "cpp_defense/infrastructure/runtime_path.hpp"
#include "cpp_defense/ui/cli_app.hpp"

int main(int argc, char* argv[]) {
  cpp_defense::CliApp app(std::cin, std::cout, std::cerr,
                          cpp_defense::ResolveRuntimeRoot());
  return app.Run(argc, argv);
}
