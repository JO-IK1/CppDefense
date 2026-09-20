#include "cpp_defense/worker/worker.hpp"
#include "protocol.hpp"
#include "workspace.hpp"
#include "cpp_defense/core/candidate_picker.hpp"
#include "cpp_defense/core/sha256.hpp"
#include "cpp_defense/core/source_parser.hpp"
#include "cpp_defense/infrastructure/project_scanner.hpp"
#include "cpp_defense/infrastructure/session_id.hpp"
#include <algorithm>
#include <array>
#include <map>
#include <set>

namespace cpp_defense::worker {
namespace {
struct Analysis {
  std::vector<CodeEntityInfo> entities;
  std::map<fs::path, std::string> sources;
  std::string fingerprint;
};

Analysis Analyze(const fs::path& project, const Json& payload) {
  const auto files = ValidateTree(project);
  Require(fs::is_regular_file(Resolve(project, "CMakeLists.txt")),
          "INVALID_PROJECT", "Project requires CMakeLists.txt");
  std::string fingerprint;
  for (const auto& file : files) {
    const auto relative = file.lexically_relative(project).generic_string();
    fingerprint += std::to_string(relative.size()) + ":" + relative + Sha256(Read(file));
  }

  ProjectScannerOptions options;
  options.excluded_directory_names = {".git", "build", ".idea", ".vscode"};
  if (payload.contains("parser_options")) {
    const auto& config = payload["parser_options"];
    if (config.contains("ignored_directories")) {
      options.excluded_directory_names = config["ignored_directories"].get<std::vector<std::string>>();
    }
    if (config.contains("source_extensions")) {
      options.source_extensions = config["source_extensions"].get<std::vector<std::string>>();
    }
  }
  const auto paths = ProjectScanner(options).FindSourceFiles(project);
  if (!paths) throw Error("INVALID_PROJECT", "Source scan failed", "invalid_project");
  Analysis result;
  result.fingerprint = Sha256(fingerprint);
  const SimpleSourceParser parser;
  for (const auto& path : *paths) {
    const auto relative = path.lexically_relative(project);
    auto source = Read(path);
    const auto parsed = parser.Parse(source, relative);
    if (!parsed) throw Error("PARSE_ERROR", "Source could not be parsed", "invalid_project");
    for (auto entity : *parsed) {
      if (entity.type == CodeEntityType::kFunction) result.entities.push_back(std::move(entity));
    }
    result.sources.emplace(relative, std::move(source));
  }
  Require(result.entities.size() <= 10000, "PROJECT_LIMIT", "Too many function candidates");
  std::sort(result.entities.begin(), result.entities.end(), CandidatePriorityCompare{});
  return result;
}

Json Candidate(const CodeEntityInfo& entity, const std::string& source) {
  Require(entity.start_offset <= entity.body_start_offset &&
          entity.body_start_offset < entity.body_end_offset &&
          entity.body_end_offset <= source.size(),
          "INVALID_RANGE", "Invalid parser offsets");
  const auto signature = source.substr(entity.start_offset,
                                      entity.body_start_offset-entity.start_offset);
  Require(!signature.empty() && signature.size() <= 16384 && entity.name.size() <= 1024,
          "PROJECT_LIMIT", "Function metadata exceeds protocol limits");
  const auto body = source.substr(entity.body_start_offset+1,
                                 entity.body_end_offset-entity.body_start_offset-2);
  return {{"function_name", entity.name}, {"file_path", entity.file_path.generic_string()},
          {"signature", signature}, {"signature_begin", entity.start_offset},
          {"body_begin", entity.body_start_offset}, {"body_end", entity.body_end_offset},
          {"begin_line", entity.body_start_line}, {"end_line", entity.body_end_line},
          {"line_count", entity.body_line_count()}, {"source_sha256", Sha256(source)},
          {"original_body_sha256", Sha256(body)}};
}

std::string MaskedSource(const CodeEntityInfo& entity,
                         const std::string& source) {
  Require(entity.body_start_offset + 1 <= entity.body_end_offset - 1 &&
              entity.body_end_offset <= source.size(),
          "INVALID_RANGE", "Invalid selected body range");
  std::string masked = source;
  constexpr std::string_view marker = "/* TODO */";
  for (std::size_t i = entity.body_start_offset + 1;
       i < entity.body_end_offset - 1; ++i) {
    if (masked[i] == '\n' || masked[i] == '\r') continue;
    masked[i] = ' ';
  }
  for (std::size_t i = entity.body_start_offset + 1;
       i < entity.body_end_offset - 1;) {
    if (masked[i] == '\n' || masked[i] == '\r') {
      ++i;
      continue;
    }
    const auto begin = i;
    while (i < entity.body_end_offset - 1 && masked[i] != '\n' &&
           masked[i] != '\r') {
      ++i;
    }
    if (i - begin >= marker.size()) {
      std::copy(marker.begin(), marker.end(), masked.begin() + begin);
      break;
    }
  }
  return masked;
}

bool ContainsPath(const fs::path& parent, const fs::path& child) {
  auto a = parent.begin();
  auto b = child.begin();
  for (; a != parent.end(); ++a, ++b) {
    if (b == child.end() || *a != *b) return false;
  }
  return true;
}

Json Materialize(const fs::path& session, const fs::path& project, const Json& p) {
  ValidateTree(project);
  const auto output = Resolve(session, p["output_root"].get<std::string>(), false);
  Require(!ContainsPath(project, output) && !ContainsPath(output, project),
          "INVALID_PATH", "Input and output paths must not overlap");
  Require(fs::is_directory(output.parent_path()), "INVALID_PATH", "Output parent must exist");
  Require(!fs::exists(output), "OUTPUT_EXISTS", "Output already exists");
  const auto& selected = p["selected_function"];
  const auto relative = selected["file_path"].get<std::string>();
  const auto input_file = Resolve(project, relative);
  const auto source = Read(input_file);
  if (Sha256(source) != selected["source_sha256"].get<std::string>()) {
    throw Error("SOURCE_CHANGED", "Selected source digest no longer matches", "conflict");
  }
  const auto begin = selected["body_begin"].get<std::size_t>();
  const auto end = selected["body_end"].get<std::size_t>();
  const auto signature = selected["signature_begin"].get<std::size_t>();
  Require(signature <= begin && begin < end && end <= source.size() &&
          source[begin] == '{' && source[end-1] == '}',
          "INVALID_RANGE", "Invalid selected body range");
  const SimpleSourceParser parser;
  const auto parsed = parser.Parse(source, relative);
  Require(parsed.has_value(), "INVALID_PROJECT", "Selected source cannot be parsed");
  const auto match = std::find_if(parsed->begin(), parsed->end(), [&](const auto& entity) {
    return entity.type == CodeEntityType::kFunction &&
           entity.start_offset == signature && entity.body_start_offset == begin &&
           entity.body_end_offset == end &&
           entity.name == selected["function_name"].get<std::string>();
  });
  Require(match != parsed->end(), "INVALID_RANGE", "Selection is not a parsed function");
  const auto answer = p["answer"].get<std::string>();
  Require(parser.ValidateBody(answer, relative).has_value(),
          "INVALID_ANSWER", "Answer must stay inside the original function body");

  TemporaryTree staging(Resolve(session, ".attempt-" + NewSessionId(), false));
  fs::copy(project, staging.path(), fs::copy_options::recursive);
  auto modified = source;
  modified.replace(begin+1, end-begin-2, answer);
  Write(Resolve(staging.path(), relative), modified);
  staging.Commit(output);
  return {{"output_root", p["output_root"]}, {"modified_file", relative},
          {"answer_sha256", Sha256(answer)}};
}

Json Execute(const fs::path& workspace, const Json& r) {
  Require(fs::is_directory(workspace), "INVALID_PATH", "Workspace root is missing");
  const auto root = fs::canonical(workspace);
  const auto session = Resolve(root, r["session_id"].get<std::string>());
  Require(fs::is_directory(session), "INVALID_PATH", "Runner must stage the session first");
  SessionLock lock(session);
  const auto& p = r["payload"];
  const auto project = Resolve(session, p["project_root"].get<std::string>());
  if (r["command"] == "materialize_attempt") return Materialize(session, project, p);

  const auto analysis = Analyze(project, p);
  if (r["command"] == "analyze_project") {
    auto candidates = Json::array();
    for (const auto& entity : analysis.entities) {
      candidates.push_back(Candidate(entity, analysis.sources.at(entity.file_path)));
    }
    return {{"source_file_count", analysis.sources.size()},
            {"function_count", candidates.size()}, {"candidates", candidates}};
  }

  CandidatePicker picker(Seed(p["seed"]));
  const auto choice = picker.Pick(analysis.entities, p["top_n"].get<std::size_t>(),
                                  CandidateSelectionMode::kFunctionsOnly);
  if (!choice) {
    throw Error("NO_FUNCTION_CANDIDATES", "No suitable functions",
                "invalid_project");
  }
  auto candidates = Json::array();
  for (std::size_t i = 0; i < choice->candidates.size(); ++i) {
    const auto& entity = choice->candidates[i];
    candidates.push_back(Candidate(entity, analysis.sources.at(entity.file_path)));
  }
  const auto& selected = choice->candidates[choice->selected_index];
  const Json result{
      {"candidates", candidates},
      {"selected_index", choice->selected_index},
      {"selected_function", candidates.at(choice->selected_index)},
      {"masked_source", MaskedSource(selected, analysis.sources.at(selected.file_path))}};
  const auto key = Sha256(p.dump() + analysis.fingerprint);
  const auto state_path = Resolve(session, "defense-state.json", false);
  if (fs::exists(state_path)) {
    const auto saved = Parse(Read(state_path));
    if (saved.value("preparation_key", "") != key || saved.at("result") != result) {
      throw Error("SESSION_CONFLICT", "Session already prepared with different inputs", "conflict");
    }
    return saved.at("result");
  }
  SaveState(session, {{"protocol_version", "1.0"}, {"preparation_key", key},
                     {"session_id", r["session_id"]}, {"seed", p["seed"]},
                     {"project_sha256", analysis.fingerprint}, {"result", result}});
  return result;
}
}

int Run(std::istream& input, std::ostream& output, const fs::path& workspace) {
  Json envelope{{"protocol_version", "1.0"}, {"request_id", nullptr},
                {"session_id", nullptr}, {"command", nullptr}};
  Json response;
  try {
    std::string bytes;
    std::array<char, 8192> buffer{};
    while (input.read(buffer.data(), buffer.size()) || input.gcount()) {
      bytes.append(buffer.data(), static_cast<std::size_t>(input.gcount()));
      Require(bytes.size() <= 8*1024*1024, "REQUEST_TOO_LARGE", "Request exceeds 8 MiB");
    }
    const auto request = Parse(bytes);
    if (request.is_object() && request.contains("request_id") && request.contains("session_id") &&
        request["request_id"].is_string() && request["session_id"].is_string() &&
        IsSessionId(request["request_id"].get<std::string>()) &&
        IsSessionId(request["session_id"].get<std::string>()) &&
        request.contains("command") &&
        (request["command"] == "analyze_project" || request["command"] == "prepare_defense" ||
         request["command"] == "materialize_attempt")) {
      for (const auto* key : {"request_id", "session_id", "command"}) envelope[key] = request[key];
    }
    ValidateRequest(request);
    response = envelope;
    response["result"] = Execute(workspace, request);
    response["status"] = "ok";
    Require(response.dump().size() <= 16*1024*1024, "RESPONSE_TOO_LARGE", "Response exceeds 16 MiB");
  } catch (const Error& error) {
    response = Failure(envelope, error);
  } catch (const Json::exception&) {
    response = Failure(envelope, Error("INVALID_JSON", "Malformed JSON or invalid state document"));
  } catch (const fs::filesystem_error&) {
    response = Failure(envelope, Error("IO_ERROR", "Workspace filesystem operation failed", "internal", true));
  } catch (const std::exception&) {
    response = Failure(envelope, Error("INTERNAL_ERROR", "Worker operation failed", "internal", true));
  }
  output << response.dump(-1, ' ', false, Json::error_handler_t::replace) << '\n';
  return output ? 0 : 74;
}
}  // namespace cpp_defense::worker
