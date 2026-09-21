#include "protocol.hpp"
#include "cpp_defense/infrastructure/session_id.hpp"
#include <algorithm>
#include <charconv>
#include <limits>
#include <regex>
#include <set>
#include <vector>

namespace cpp_defense::worker {
void Require(bool condition, std::string_view code, std::string_view message) {
  if (!condition) throw Error(std::string(code), std::string(message));
}

namespace {
void Fields(const Json& object, std::initializer_list<const char*> required,
            std::initializer_list<const char*> optional = {}) {
  Require(object.is_object(), "INVALID_REQUEST", "Expected an object");
  for (const auto* key : required) {
    Require(object.contains(key), "INVALID_REQUEST", "Missing required field");
  }
  for (auto it = object.begin(); it != object.end(); ++it) {
    const auto matches = [&](const char* key) { return it.key() == key; };
    Require(std::any_of(required.begin(), required.end(), matches) ||
            std::any_of(optional.begin(), optional.end(), matches),
            "INVALID_REQUEST", "Unknown field");
  }
}
bool Text(const Json& j, std::size_t minimum, std::size_t maximum) {
  return j.is_string() && j.get_ref<const std::string&>().size() >= minimum &&
         j.get_ref<const std::string&>().size() <= maximum;
}
bool Number(const Json& j, std::uint64_t minimum, std::uint64_t maximum) {
  if (!j.is_number_integer()) return false;
  if (j.is_number_integer() && !j.is_number_unsigned() && j.get<std::int64_t>() < 0) return false;
  const auto value = j.get<std::uint64_t>();
  return value >= minimum && value <= maximum;
}
void Path(const Json& value) {
  Require(Text(value, 1, 1024) && RelativePath(value.get<std::string>()),
          "INVALID_PATH", "Expected a safe workspace-relative path");
}
}

bool RelativePath(std::string_view path) {
  if (path.empty() || path.front() == '/' || path.back() == '/') return false;
  std::size_t start = 0;
  for (std::size_t i = 0; i <= path.size(); ++i) {
    if (i < path.size()) {
      const auto ch = static_cast<unsigned char>(path[i]);
      if (ch < 32 || ch == 127 || ch == '\\' || ch == ':') return false;
    }
    if (i == path.size() || path[i] == '/') {
      const auto part = path.substr(start, i-start);
      if (part.empty() || part == "." || part == "..") return false;
      start = i+1;
    }
  }
  return true;
}

Json Parse(std::string_view text) {
  std::vector<std::set<std::string>> keys;
  return Json::parse(text, [&](int depth, Json::parse_event_t event, Json& value) {
    Require(depth <= 32, "INVALID_REQUEST", "JSON nesting limit exceeded");
    if (event == Json::parse_event_t::object_start) keys.emplace_back();
    if (event == Json::parse_event_t::key) {
      Require(keys.back().insert(value.get<std::string>()).second,
              "INVALID_REQUEST", "Duplicate JSON key");
    }
    if (event == Json::parse_event_t::object_end) keys.pop_back();
    return true;
  });
}

std::uint64_t Seed(const Json& value) {
  Require(value.is_string(), "INVALID_REQUEST", "Seed must be a decimal string");
  const auto& text = value.get_ref<const std::string&>();
  Require(!text.empty() && text.size() <= 20 &&
          (text.size() == 1 || text.front() != '0'),
          "INVALID_REQUEST", "Invalid uint64 seed");
  std::uint64_t result{};
  const auto [end, ec] = std::from_chars(text.data(), text.data()+text.size(), result);
  Require(ec == std::errc{} && end == text.data()+text.size(),
          "INVALID_REQUEST", "Invalid uint64 seed");
  return result;
}

void ValidateRequest(const Json& r) {
  Fields(r, {"protocol_version", "request_id", "session_id", "command", "payload"});
  Require(r["protocol_version"] == "1.0", "UNSUPPORTED_PROTOCOL", "Expected protocol 1.0");
  for (const auto* key : {"request_id", "session_id"}) {
    Require(r[key].is_string() && IsSessionId(r[key].get<std::string>()),
            "INVALID_REQUEST", "Invalid UUID");
  }
  Require(r["command"] == "analyze_project" || r["command"] == "prepare_defense" ||
          r["command"] == "materialize_attempt", "UNKNOWN_COMMAND", "Unknown command");
  const auto& p = r["payload"];
  if (r["command"] == "analyze_project") {
    Fields(p, {"project_root"}, {"parser_options"});
  } else if (r["command"] == "prepare_defense") {
    Fields(p, {"project_root", "top_n", "seed"}, {"parser_options", "selected_index"});
    Require(Number(p["top_n"], 1, 50), "INVALID_REQUEST", "top_n must be 1..50");
    if (p.contains("selected_index")) {
      Require(Number(p["selected_index"], 0, 49), "INVALID_REQUEST",
              "selected_index must be 0..49");
    }
    (void)Seed(p["seed"]);
  } else {
    Fields(p, {"project_root", "output_root", "selected_function", "answer"});
    Path(p["output_root"]);
    Require(Text(p["answer"], 0, 1048576), "INVALID_REQUEST", "Answer exceeds 1 MiB");
    const auto& f = p["selected_function"];
    Fields(f, {"function_name", "file_path", "signature_begin", "body_begin",
               "body_end", "source_sha256"});
    Require(Text(f["function_name"], 1, 1024), "INVALID_REQUEST", "Invalid function name");
    Path(f["file_path"]);
    for (const auto* key : {"signature_begin", "body_begin", "body_end"}) {
      Require(Number(f[key], 0, 16*1024*1024), "INVALID_REQUEST", "Invalid byte offset");
    }
    Require(Text(f["source_sha256"], 64, 64) &&
            std::regex_match(f["source_sha256"].get<std::string>(), std::regex("[a-f0-9]{64}")),
            "INVALID_REQUEST", "Invalid source SHA-256");
  }
  Path(p["project_root"]);
  if (p.contains("parser_options")) {
    const auto& options = p["parser_options"];
    Fields(options, {}, {"source_extensions", "ignored_directories"});
    for (auto it = options.begin(); it != options.end(); ++it) {
      const bool extensions = it.key() == "source_extensions";
      Require(it->is_array() && it->size() <= (extensions ? 32 : 64) &&
              (!extensions || !it->empty()), "INVALID_REQUEST", "Invalid parser options");
      std::set<std::string> unique;
      for (const auto& item : *it) {
        Require(Text(item, 1, extensions ? 11 : 128), "INVALID_REQUEST", "Invalid parser option");
        const auto value = item.get<std::string>();
        Require(unique.insert(value).second, "INVALID_REQUEST", "Duplicate parser option");
        Require(extensions ? std::regex_match(value, std::regex("\\.[A-Za-z0-9]{1,10}"))
                           : (RelativePath(value) && value.find('/') == std::string::npos),
                "INVALID_REQUEST", "Invalid parser option syntax");
      }
    }
  }
}

Json Failure(const Json& envelope, const Error& error) {
  Json result = envelope;
  result["status"] = "error";
  result["error"] = {{"code", error.code}, {"message", error.what()},
                     {"category", error.category}, {"retryable", error.retryable}};
  return result;
}
}  // namespace cpp_defense::worker
