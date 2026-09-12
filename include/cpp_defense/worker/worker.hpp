#pragma once

#include <filesystem>
#include <istream>
#include <ostream>

namespace cpp_defense::worker {
// One request and one JSON response. Does not execute student code.
int Run(std::istream& input, std::ostream& output,
        const std::filesystem::path& workspace_root);
}  // namespace cpp_defense::worker
