#include "cpp_defense/core/sha256.hpp"
#include "cpp_defense/worker/worker.hpp"

#include <chrono>
#include <filesystem>
#include <fstream>
#include <future>
#include <iostream>
#include <sstream>
#include <string>
#include <string_view>

#include <nlohmann/json.hpp>

namespace {

namespace fs = std::filesystem;
using Json = nlohmann::json;

constexpr std::string_view kSessionOne =
    "0199a123-4568-7abc-8def-0123456789ab";
constexpr std::string_view kSessionTwo =
    "0199a123-4569-7abc-8def-0123456789ab";

class TemporaryDirectory {
 public:
  TemporaryDirectory() {
    path_ = fs::temp_directory_path() /
            ("cpp-defense-worker-test-" +
             std::to_string(std::chrono::steady_clock::now()
                                .time_since_epoch()
                                .count()));
    fs::create_directories(path_);
  }

  ~TemporaryDirectory() {
    std::error_code ignored;
    fs::remove_all(path_, ignored);
  }

  const fs::path& path() const noexcept { return path_; }

 private:
  fs::path path_;
};

void Write(const fs::path& path, std::string_view contents) {
  fs::create_directories(path.parent_path());
  std::ofstream output(path, std::ios::binary | std::ios::trunc);
  output << contents;
}

std::string Read(const fs::path& path) {
  std::ifstream input(path, std::ios::binary);
  return {std::istreambuf_iterator<char>(input),
          std::istreambuf_iterator<char>()};
}

bool Expect(bool condition, std::string_view message) {
  if (!condition) {
    std::cerr << "FAILED: " << message << '\n';
  }
  return condition;
}

fs::path CreateProject(const fs::path& root, std::string_view session_id) {
  const fs::path project = root / session_id / "project";
  Write(project / "CMakeLists.txt",
        "cmake_minimum_required(VERSION 3.20)\n"
        "project(Sample LANGUAGES CXX)\n"
        "add_executable(sample src/math.cpp)\n");
  Write(project / "src/math.cpp",
        "int Add(int a, int b) {\n"
        "  int result = a + b;\n"
        "  return result;\n"
        "}\n\n"
        "int Multiply(int a, int b) {\n"
        "  int result = a * b;\n"
        "  return result;\n"
        "}\n");
  return project;
}

Json Request(std::string_view command, std::string_view session_id,
             Json payload, std::string_view request_suffix = "01") {
  return {
      {"protocol_version", "1.0"},
      {"request_id", std::string("0199a123-4567-7abc-8def-0123456789") +
                         std::string(request_suffix)},
      {"session_id", session_id},
      {"command", command},
      {"payload", std::move(payload)},
  };
}

Json Run(const fs::path& root, const Json& request) {
  std::istringstream input(request.dump());
  std::ostringstream output;
  const int exit_code = cpp_defense::worker::Run(input, output, root);
  if (exit_code != 0) {
    throw std::runtime_error("worker failed to write its response");
  }
  return Json::parse(output.str());
}

Json Prepare(const fs::path& root, std::string_view session_id,
             std::string_view seed = "42", std::string_view suffix = "01") {
  return Run(root, Request("prepare_defense", session_id,
                           {{"project_root", "project"},
                            {"top_n", 2},
                            {"seed", seed}},
                           suffix));
}

bool TestSha256() {
  return Expect(
      cpp_defense::Sha256("abc") ==
          "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
      "SHA-256 matches the standard abc vector");
}

bool TestAnalyzeAndPrepare() {
  TemporaryDirectory temporary;
  const fs::path project = CreateProject(temporary.path(), kSessionOne);

  const Json analyze = Run(
      temporary.path(),
      Request("analyze_project", kSessionOne, {{"project_root", "project"}}));
  bool passed = Expect(analyze.at("status") == "ok", "analysis succeeds");
  passed &= Expect(analyze.at("result").at("source_file_count") == 1,
                   "analysis reports source files");
  passed &= Expect(analyze.at("result").at("function_count") == 2,
                   "analysis reports functions");

  const Json first = Prepare(temporary.path(), kSessionOne, "42", "02");
  const Json second = Prepare(temporary.path(), kSessionOne, "42", "03");
  passed &= Expect(first.at("status") == "ok", "preparation succeeds");
  passed &= Expect(first.at("result") == second.at("result"),
                   "same project and seed reproduce the same result");
  passed &= Expect(
      fs::is_regular_file(temporary.path() / kSessionOne /
                          "defense-state.json"),
      "preparation state is saved");
  passed &= Expect(Read(project / "src/math.cpp").find("a + b") !=
                       std::string::npos,
                   "analysis never modifies the input project");
  const auto& selected = first.at("result").at("selected_function");
  const auto original = Read(project / selected.at("file_path").get<std::string>());
  const auto body_begin = selected.at("body_begin").get<std::size_t>();
  const auto body_end = selected.at("body_end").get<std::size_t>();
  const auto original_body = original.substr(body_begin + 1, body_end - body_begin - 2);
  const auto masked = first.at("result").at("masked_source").get<std::string>();
  passed &= Expect(masked.find(original_body) == std::string::npos,
                   "prepared source hides the selected implementation");
  passed &= Expect(masked.find("/* TODO */") != std::string::npos,
                   "prepared source marks the missing implementation");

  const Json conflict = Prepare(temporary.path(), kSessionOne, "43", "04");
  passed &= Expect(conflict.at("status") == "error" &&
                       conflict.at("error").at("code") == "SESSION_CONFLICT",
                   "different preparation cannot overwrite saved state");
  return passed;
}

bool TestMaterializeBodyWithoutOuterBraces() {
  TemporaryDirectory temporary;
  const fs::path project = CreateProject(temporary.path(), kSessionOne);
  fs::create_directories(temporary.path() / kSessionOne / "attempts");

  const Json prepared = Prepare(temporary.path(), kSessionOne);
  if (!Expect(prepared.at("status") == "ok", "preparation succeeds")) {
    return false;
  }

  const Json selected = prepared.at("result").at("selected_function");
  const std::string answer = "\n  return a - b;\n";
  const Json materialized = Run(
      temporary.path(),
      Request("materialize_attempt", kSessionOne,
              {{"project_root", "project"},
               {"output_root", "attempts/one"},
               {"selected_function",
                {{"function_name", selected.at("function_name")},
                 {"file_path", selected.at("file_path")},
                 {"signature_begin", selected.at("signature_begin")},
                 {"body_begin", selected.at("body_begin")},
                 {"body_end", selected.at("body_end")},
                 {"source_sha256", selected.at("source_sha256")}}},
               {"answer", answer}},
              "05"));

  const fs::path output_file = temporary.path() / kSessionOne / "attempts/one" /
                               selected.at("file_path").get<std::string>();
  bool passed = Expect(materialized.at("status") == "ok",
                       "materialization succeeds");
  passed &= Expect(Read(output_file).find("{\n  return a - b;\n}") !=
                       std::string::npos,
                   "worker preserves outer braces around body-only answer");
  passed &= Expect(Read(project / "src/math.cpp").find("return a - b") ==
                       std::string::npos,
                   "materialization leaves input immutable");
  passed &= Expect(materialized.at("result").at("answer_sha256") ==
                       cpp_defense::Sha256(answer),
                   "materialization reports the answer digest");
  return passed;
}

bool TestInvalidInputsAndSourceChange() {
  TemporaryDirectory temporary;
  const fs::path project = CreateProject(temporary.path(), kSessionOne);
  fs::create_directories(temporary.path() / kSessionOne / "attempts");

  const Json unsafe = Run(
      temporary.path(),
      Request("analyze_project", kSessionOne,
              {{"project_root", "../project"}}));
  bool passed = Expect(unsafe.at("status") == "error" &&
                           unsafe.at("error").at("code") == "INVALID_PATH",
                       "parent traversal is rejected");

  const Json prepared = Prepare(temporary.path(), kSessionOne, "42", "02");
  const Json selected = prepared.at("result").at("selected_function");
  Write(project / selected.at("file_path").get<std::string>(),
        Read(project / selected.at("file_path").get<std::string>()) +
            "\n// changed\n");
  const Json changed = Run(
      temporary.path(),
      Request("materialize_attempt", kSessionOne,
              {{"project_root", "project"},
               {"output_root", "attempts/changed"},
               {"selected_function",
                {{"function_name", selected.at("function_name")},
                 {"file_path", selected.at("file_path")},
                 {"signature_begin", selected.at("signature_begin")},
                 {"body_begin", selected.at("body_begin")},
                 {"body_end", selected.at("body_end")},
                 {"source_sha256", selected.at("source_sha256")}}},
               {"answer", "return 0;"}},
              "03"));
  passed &= Expect(changed.at("status") == "error" &&
                       changed.at("error").at("code") == "SOURCE_CHANGED",
                   "changed source is rejected by digest");
  passed &= Expect(!fs::exists(temporary.path() / kSessionOne /
                              "attempts/changed"),
                   "failed materialization leaves no output tree");
  return passed;
}

bool TestParallelSessionsAreIsolated() {
  TemporaryDirectory temporary;
  CreateProject(temporary.path(), kSessionOne);
  CreateProject(temporary.path(), kSessionTwo);

  auto first = std::async(std::launch::async, [&] {
    return Prepare(temporary.path(), kSessionOne, "7", "01");
  });
  auto second = std::async(std::launch::async, [&] {
    return Prepare(temporary.path(), kSessionTwo, "7", "02");
  });

  const Json first_result = first.get();
  const Json second_result = second.get();
  return Expect(first_result.at("status") == "ok" &&
                    second_result.at("status") == "ok",
                "parallel sessions both complete") &&
         Expect(fs::is_regular_file(temporary.path() / kSessionOne /
                                    "defense-state.json") &&
                    fs::is_regular_file(temporary.path() / kSessionTwo /
                                        "defense-state.json"),
                "parallel sessions persist separate state");
}

}  // namespace

int main() {
  bool passed = true;
  passed &= TestSha256();
  passed &= TestAnalyzeAndPrepare();
  passed &= TestMaterializeBodyWithoutOuterBraces();
  passed &= TestInvalidInputsAndSourceChange();
  passed &= TestParallelSessionsAreIsolated();
  return passed ? 0 : 1;
}
