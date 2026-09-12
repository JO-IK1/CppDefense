#include "cpp_defense/worker/worker.hpp"
#include <iostream>
#include <string_view>

int main(int argc, char* argv[]) {
  if (argc != 3 || std::string_view(argv[1]) != "--workspace") {
    std::cerr << "Usage: cpp-defense-worker --workspace <runner-owned-directory>\n";
    return 64;
  }
  return cpp_defense::worker::Run(std::cin, std::cout, argv[2]);
}
