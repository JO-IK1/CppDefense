#include <filesystem>
#include <iostream>
#include <string>
#include <string_view>

#include "cpp_defense/core/code_entity_info.hpp"
#include "cpp_defense/core/parse_error.hpp"
#include "cpp_defense/infrastructure/simple_source_parser.hpp"

namespace {

using cpp_defense::CodeEntityInfo;
using cpp_defense::CodeEntityType;
using cpp_defense::ParseErrorType;
using cpp_defense::SimpleSourceParser;

constexpr int kSuccessExitCode = 0;
constexpr int kFailureExitCode = 1;

bool Expect(bool condition, std::string_view message) {
  if (condition) {
    return true;
  }

  std::cerr << "FAILED: " << message << '\n';
  return false;
}

bool ExpectEntity(const CodeEntityInfo& entity,
                  CodeEntityType expected_type,
                  std::string_view expected_name) {
  return Expect(entity.type == expected_type, "entity type is correct") &&
         Expect(entity.name == expected_name, "entity name is correct");
}

std::string_view GetEntitySource(std::string_view source,
                                 const CodeEntityInfo& entity) {
  return source.substr(entity.start_offset,
                       entity.end_offset - entity.start_offset);
}

std::string_view GetBodySource(std::string_view source,
                               const CodeEntityInfo& entity) {
  return source.substr(
      entity.body_start_offset,
      entity.body_end_offset - entity.body_start_offset);
}

bool TestEmptySource() {
  const SimpleSourceParser parser;
  const auto result = parser.Parse("", "example.cpp");

  return Expect(result.has_value(), "empty source parses successfully") &&
         Expect(result->empty(), "empty source contains no entities");
}

bool TestSimpleFunction() {
  const std::string source =
      "int Sum(int a, int b) {\n"
      "  return a + b;\n"
      "}\n";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "example.cpp");

  if (!Expect(result.has_value(), "function parses successfully") ||
      !Expect(result->size() == 1, "one function is found")) {
    return false;
  }

  return ExpectEntity(
      result->front(), CodeEntityType::kFunction, "Sum");
}

bool TestSimpleClass() {
  const std::string source =
      "class Player {\n"
      " public:\n"
      "  int health = 100;\n"
      "};\n";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "player.hpp");

  if (!Expect(result.has_value(), "class parses successfully") ||
      !Expect(result->size() == 1, "one class is found")) {
    return false;
  }

  return ExpectEntity(
      result->front(), CodeEntityType::kClass, "Player");
}

bool TestSimpleStruct() {
  const std::string source =
      "struct Config {\n"
      "  int width;\n"
      "  int height;\n"
      "};\n";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "config.hpp");

  if (!Expect(result.has_value(), "struct parses successfully") ||
      !Expect(result->size() == 1, "one struct is found")) {
    return false;
  }

  return ExpectEntity(
      result->front(), CodeEntityType::kStruct, "Config");
}

bool TestClassWithMethod() {
  const std::string source =
      "class Player {\n"
      " public:\n"
      "  void Move() {\n"
      "  }\n"
      "};\n";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "player.hpp");

  if (!Expect(result.has_value(), "class with method parses") ||
      !Expect(result->size() == 2,
              "class and method are both found")) {
    return false;
  }

  return ExpectEntity(
             (*result)[0], CodeEntityType::kClass, "Player") &&
         ExpectEntity(
             (*result)[1], CodeEntityType::kFunction, "Player::Move");
}

bool TestStructWithMethod() {
  const std::string source =
      "struct Counter {\n"
      "  int Get() const {\n"
      "    return 42;\n"
      "  }\n"
      "};\n";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "counter.hpp");

  if (!Expect(result.has_value(), "struct with method parses") ||
      !Expect(result->size() == 2,
              "struct and method are both found")) {
    return false;
  }

  return ExpectEntity(
             (*result)[0], CodeEntityType::kStruct, "Counter") &&
         ExpectEntity(
             (*result)[1], CodeEntityType::kFunction, "Counter::Get");
}

bool TestClassWithInheritance() {
  const std::string source =
      "class Player : public Entity {\n"
      " public:\n"
      "  void Update() {\n"
      "  }\n"
      "};\n";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "player.hpp");

  if (!Expect(result.has_value(), "inherited class parses") ||
      !Expect(result->size() == 2,
              "class and inherited class method are found")) {
    return false;
  }

  return ExpectEntity(
             (*result)[0], CodeEntityType::kClass, "Player") &&
         ExpectEntity(
             (*result)[1], CodeEntityType::kFunction, "Player::Update");
}

bool TestNestedClasses() {
  const std::string source =
      "class Outer {\n"
      " public:\n"
      "  class Inner {\n"
      "  };\n"
      "};\n";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "example.hpp");

  if (!Expect(result.has_value(), "nested classes parse") ||
      !Expect(result->size() == 2,
              "outer and inner classes are found")) {
    return false;
  }

  return ExpectEntity(
             (*result)[0], CodeEntityType::kClass, "Outer") &&
         ExpectEntity(
             (*result)[1], CodeEntityType::kClass, "Inner");
}

bool TestMultipleEntityTypes() {
  const std::string source =
      "struct Config {\n"
      "  int value;\n"
      "};\n"
      "\n"
      "class Player {\n"
      " public:\n"
      "  void Move() {\n"
      "  }\n"
      "};\n"
      "\n"
      "int Sum(int a, int b) {\n"
      "  return a + b;\n"
      "}\n";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "example.cpp");

  if (!Expect(result.has_value(), "mixed entities parse") ||
      !Expect(result->size() == 4,
              "all supported entities are found")) {
    return false;
  }

  return ExpectEntity(
             (*result)[0], CodeEntityType::kStruct, "Config") &&
         ExpectEntity(
             (*result)[1], CodeEntityType::kClass, "Player") &&
         ExpectEntity(
             (*result)[2], CodeEntityType::kFunction, "Player::Move") &&
         ExpectEntity(
             (*result)[3], CodeEntityType::kFunction, "Sum");
}

bool TestNamespaceIgnored() {
  const std::string source =
      "namespace app {\n"
      "\n"
      "class Player {\n"
      "};\n"
      "\n"
      "int Run() {\n"
      "  return 0;\n"
      "}\n"
      "\n"
      "}\n";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "example.cpp");

  if (!Expect(result.has_value(), "namespace source parses") ||
      !Expect(result->size() == 2,
              "namespace itself is not returned")) {
    return false;
  }

  return ExpectEntity(
             (*result)[0], CodeEntityType::kClass, "Player") &&
         ExpectEntity(
             (*result)[1], CodeEntityType::kFunction, "Run");
}

bool TestEnumClass() {
  const std::string source =
      "enum class State {\n"
      "  kIdle,\n"
      "  kRunning,\n"
      "};\n";

  const std::string_view expected_entity =
      "enum class State {\n"
      "  kIdle,\n"
      "  kRunning,\n"
      "};";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "state.hpp");

  if (!Expect(result.has_value(), "enum class parses") ||
      !Expect(result->size() == 1, "enum class is found")) {
    return false;
  }

  return ExpectEntity(
             result->front(), CodeEntityType::kEnumClass, "State") &&
         Expect(GetEntitySource(source, result->front()) == expected_entity,
                "enum class entity range is exact");
}

bool TestEnumClassWithUnderlyingType() {
  const std::string source =
      "enum class ResultType : unsigned char {\n"
      "  kOk,\n"
      "  kError,\n"
      "};\n";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "result.hpp");

  return Expect(result.has_value(), "typed enum class parses") &&
         Expect(result->size() == 1, "typed enum class is found") &&
         ExpectEntity(result->front(), CodeEntityType::kEnumClass,
                      "ResultType");
}

bool TestUnscopedEnumIgnored() {
  const std::string source =
      "enum State {\n"
      "  kIdle,\n"
      "  kRunning,\n"
      "};\n";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "state.hpp");

  return Expect(result.has_value(), "unscoped enum parses") &&
         Expect(result->empty(), "unscoped enum remains unsupported");
}

bool TestForwardDeclarationsIgnored() {
  const std::string source =
      "class Player;\n"
      "struct Config;\n"
      "void Run();\n";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "example.hpp");

  return Expect(result.has_value(), "forward declarations parse") &&
         Expect(result->empty(), "forward declarations are ignored");
}

bool TestQualifiedMethod() {
  const std::string source =
      "int Widget::Value() const {\n"
      "  return 42;\n"
      "}\n";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "widget.cpp");

  if (!Expect(result.has_value(), "qualified method parses") ||
      !Expect(result->size() == 1,
              "qualified method is found")) {
    return false;
  }

  return ExpectEntity(
      result->front(),
      CodeEntityType::kFunction,
      "Widget::Value");
}

bool TestMultilineFunction() {
  const std::string source =
      "int Sum(\n"
      "    int first,\n"
      "    int second) {\n"
      "  return first + second;\n"
      "}\n";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "example.cpp");

  if (!Expect(result.has_value(), "multiline function parses") ||
      !Expect(result->size() == 1,
              "multiline function is found")) {
    return false;
  }

  return ExpectEntity(
      result->front(), CodeEntityType::kFunction, "Sum");
}

bool TestControlStatementsIgnored() {
  const std::string source =
      "void Run() {\n"
      "  if (true) {\n"
      "  }\n"
      "\n"
      "  for (;;) {\n"
      "  }\n"
      "\n"
      "  while (false) {\n"
      "  }\n"
      "}\n";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "example.cpp");

  if (!Expect(result.has_value(), "control statements parse") ||
      !Expect(result->size() == 1,
              "control statements are not entities")) {
    return false;
  }

  return ExpectEntity(
      result->front(), CodeEntityType::kFunction, "Run");
}

bool TestLambdaIgnored() {
  const std::string source =
      "void Run() {\n"
      "  auto lambda = [](int value) {\n"
      "    return value * 2;\n"
      "  };\n"
      "}\n";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "example.cpp");

  if (!Expect(result.has_value(), "lambda source parses") ||
      !Expect(result->size() == 1,
              "lambda is not a named entity")) {
    return false;
  }

  return ExpectEntity(
      result->front(), CodeEntityType::kFunction, "Run");
}

bool TestCommentsAndLiteralsIgnored() {
  const std::string source =
      "const char* text = \"class Fake { };\";\n"
      "// struct Hidden { };\n"
      "/* class Commented { }; */\n"
      "const char brace = '}';\n"
      "const char* raw = R\"(struct Raw { };)\";\n"
      "\n"
      "class Real {\n"
      "};\n";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "example.cpp");

  if (!Expect(result.has_value(), "comments and literals parse") ||
      !Expect(result->size() == 1,
              "fake entities are ignored")) {
    return false;
  }

  return ExpectEntity(
      result->front(), CodeEntityType::kClass, "Real");
}

bool TestExactClassRange() {
  const std::string source =
      "class Player {\n"
      " public:\n"
      "  int health = 100;\n"
      "};\n";

  const std::string_view expected_entity =
      "class Player {\n"
      " public:\n"
      "  int health = 100;\n"
      "};";

  const std::string_view expected_body =
      "{\n"
      " public:\n"
      "  int health = 100;\n"
      "}";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "player.hpp");

  if (!Expect(result.has_value(), "class range source parses") ||
      !Expect(result->size() == 1, "class is found")) {
    return false;
  }

  const CodeEntityInfo& entity = result->front();

  return Expect(
             GetEntitySource(source, entity) == expected_entity,
             "class entity range contains full declaration") &&
         Expect(
             GetBodySource(source, entity) == expected_body,
             "class body range contains braces and body");
}

bool TestExactFunctionRange() {
  const std::string source =
      "int Sum(int a, int b) {\n"
      "  return a + b;\n"
      "}\n";

  const std::string_view expected_entity =
      "int Sum(int a, int b) {\n"
      "  return a + b;\n"
      "}";

  const std::string_view expected_body =
      "{\n"
      "  return a + b;\n"
      "}";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "example.cpp");

  if (!Expect(result.has_value(), "function range source parses") ||
      !Expect(result->size() == 1, "function is found")) {
    return false;
  }

  const CodeEntityInfo& entity = result->front();

  return Expect(
             GetEntitySource(source, entity) == expected_entity,
             "function entity range is exact") &&
         Expect(
             GetBodySource(source, entity) == expected_body,
             "function body range is exact");
}

bool TestEntityLines() {
  const std::string source =
      "\n"
      "\n"
      "struct Config {\n"
      "  int value;\n"
      "};\n";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "config.hpp");

  if (!Expect(result.has_value(), "line source parses") ||
      !Expect(result->size() == 1, "struct is found")) {
    return false;
  }

  const CodeEntityInfo& entity = result->front();

  return Expect(entity.start_line == 3,
                "entity start line is correct") &&
         Expect(entity.end_line == 5,
                "entity end line is correct");
}

bool TestCrlfOffsets() {
  const std::string source =
      "struct Config {\r\n"
      "  int value;\r\n"
      "};\r\n";

  const std::string_view expected_body =
      "{\r\n"
      "  int value;\r\n"
      "}";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "config.hpp");

  if (!Expect(result.has_value(), "CRLF source parses") ||
      !Expect(result->size() == 1, "CRLF struct is found")) {
    return false;
  }

  return Expect(
      GetBodySource(source, result->front()) == expected_body,
      "CRLF byte offsets remain exact");
}

bool TestConstructorInitializerList() {
  const std::string source =
      "class Iterator {\n"
      " public:\n"
      "  Iterator() : buffer_(nullptr), position_(0) {}\n"
      "};\n";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "iterator.hpp");

  if (!Expect(result.has_value(), "constructor parses") ||
      !Expect(result->size() == 2,
              "class and constructor are found")) {
    return false;
  }

  return ExpectEntity(
      (*result)[1], CodeEntityType::kFunction, "Iterator::Iterator");
}

bool TestOperators() {
  const std::string source =
      "class Iterator {\n"
      " public:\n"
      "  Iterator& operator++() { return *this; }\n"
      "  Iterator operator++(int) { return *this; }\n"
      "  int& operator[](int index) { return data_[index]; }\n"
      "  bool operator==(const Iterator&) const { return true; }\n"
      " private:\n"
      "  int data_[1]{};\n"
      "};\n";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "iterator.hpp");

  if (!Expect(result.has_value(), "operators parse") ||
      !Expect(result->size() == 5,
              "class and four operators are found")) {
    return false;
  }

  return ExpectEntity((*result)[1], CodeEntityType::kFunction,
                      "Iterator::operator++") &&
         ExpectEntity((*result)[2], CodeEntityType::kFunction,
                      "Iterator::operator++") &&
         ExpectEntity((*result)[3], CodeEntityType::kFunction,
                      "Iterator::operator[]") &&
         ExpectEntity((*result)[4], CodeEntityType::kFunction,
                      "Iterator::operator==");
}

bool TestNoexceptOperator() {
  const std::string source =
      "class Buffer {\n"
      " public:\n"
      "  Buffer& operator=(Buffer&& other) noexcept(\n"
      "      noexcept(other.Reset())) {\n"
      "    return *this;\n"
      "  }\n"
      "};\n";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "buffer.hpp");

  if (!Expect(result.has_value(), "noexcept operator parses") ||
      !Expect(result->size() == 2,
              "class and assignment operator are found")) {
    return false;
  }

  return ExpectEntity((*result)[1], CodeEntityType::kFunction,
                      "Buffer::operator=");
}

bool TestFriendOperatorRemainsFreeFunction() {
  const std::string source =
      "class Iterator {\n"
      " public:\n"
      "  friend Iterator operator+(int n, const Iterator& it) {\n"
      "    return it;\n"
      "  }\n"
      "};\n";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "iterator.hpp");

  if (!Expect(result.has_value(), "friend operator parses") ||
      !Expect(result->size() == 2,
              "class and friend operator are found")) {
    return false;
  }

  return ExpectEntity(
      (*result)[1], CodeEntityType::kFunction, "operator+");
}

bool TestControlFlowInsideFunctionDoesNotBecomeFunction() {
  const std::string source =
      "void Run() {\n"
      "  if constexpr (true) {\n"
      "    Check();\n"
      "  }\n"
      "}\n";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "example.cpp");

  return Expect(result.has_value(), "if constexpr source parses") &&
         Expect(result->size() == 1,
                "only the enclosing function is found") &&
         ExpectEntity(result->front(), CodeEntityType::kFunction, "Run");
}


bool TestUnterminatedBlockComment() {
  const std::string source =
      "class Player {\n"
      "  /* unfinished\n";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "player.hpp");

  return Expect(
      !result.has_value() &&
          result.error().type ==
              ParseErrorType::kUnterminatedBlockComment,
      "unterminated block comment produces parse error");
}

bool TestUnterminatedStringLiteral() {
  const std::string source =
      "void Run() {\n"
      "  const char* value = \"unfinished\n"
      "}\n";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "example.cpp");

  return Expect(
      !result.has_value() &&
          result.error().type ==
              ParseErrorType::kUnterminatedStringLiteral,
      "unterminated string produces parse error");
}

bool TestUnmatchedOpeningBrace() {
  const std::string source =
      "class Player {\n"
      " public:\n";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "player.hpp");

  return Expect(
      !result.has_value() &&
          result.error().type ==
              ParseErrorType::kUnmatchedOpeningBrace,
      "unmatched opening brace produces parse error");
}

bool TestUnmatchedClosingBrace() {
  const std::string source =
      "int value = 0;\n"
      "}\n";

  const SimpleSourceParser parser;
  const auto result = parser.Parse(source, "example.cpp");

  return Expect(
      !result.has_value() &&
          result.error().type ==
              ParseErrorType::kUnmatchedClosingBrace,
      "unmatched closing brace produces parse error");
}

struct TestCase {
  std::string_view name;
  bool (*function)();
};

constexpr TestCase kTestCases[] = {
    {"empty-source", TestEmptySource},
    {"simple-function", TestSimpleFunction},
    {"simple-class", TestSimpleClass},
    {"simple-struct", TestSimpleStruct},
    {"class-with-method", TestClassWithMethod},
    {"struct-with-method", TestStructWithMethod},
    {"class-with-inheritance", TestClassWithInheritance},
    {"nested-classes", TestNestedClasses},
    {"multiple-entity-types", TestMultipleEntityTypes},
    {"namespace-ignored", TestNamespaceIgnored},
    {"enum-class", TestEnumClass},
    {"enum-class-underlying-type", TestEnumClassWithUnderlyingType},
    {"unscoped-enum-ignored", TestUnscopedEnumIgnored},
    {"forward-declarations-ignored", TestForwardDeclarationsIgnored},
    {"qualified-method", TestQualifiedMethod},
    {"multiline-function", TestMultilineFunction},
    {"control-statements-ignored", TestControlStatementsIgnored},
    {"lambda-ignored", TestLambdaIgnored},
    {"comments-and-literals-ignored", TestCommentsAndLiteralsIgnored},
    {"exact-class-range", TestExactClassRange},
    {"exact-function-range", TestExactFunctionRange},
    {"entity-lines", TestEntityLines},
    {"crlf-offsets", TestCrlfOffsets},
    {"constructor-initializer-list", TestConstructorInitializerList},
    {"operators", TestOperators},
    {"noexcept-operator", TestNoexceptOperator},
    {"friend-operator-free", TestFriendOperatorRemainsFreeFunction},
    {"control-flow-inside-function", TestControlFlowInsideFunctionDoesNotBecomeFunction},
    {"unterminated-block-comment", TestUnterminatedBlockComment},
    {"unterminated-string-literal", TestUnterminatedStringLiteral},
    {"unmatched-opening-brace", TestUnmatchedOpeningBrace},
    {"unmatched-closing-brace", TestUnmatchedClosingBrace},
};

}  // namespace

int main(int argc, char* argv[]) {
  if (argc != 2) {
    std::cerr << "Expected test case name\n";
    return kFailureExitCode;
  }

  const std::string_view requested_test = argv[1];

  for (const auto& test_case : kTestCases) {
    if (test_case.name == requested_test) {
      return test_case.function()
                 ? kSuccessExitCode
                 : kFailureExitCode;
    }
  }

  std::cerr << "Unknown test case: "
            << requested_test << '\n';

  return kFailureExitCode;
}
