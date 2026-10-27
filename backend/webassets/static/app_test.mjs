import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import vm from "node:vm";

const source = await readFile(new URL("./app.js", import.meta.url), "utf8");

test("reference acceptance is explicit in student and teacher reports and never fakes stages", () => {
  const context = browserContext(null);
  context.document.createElement = tag => ({tag, children: [], append(...children) {this.children.push(...children);}});
  vm.runInContext(source, context);
  context.attempt = {outcome: "passed", attempt_no: 1, defense_id: "defense", accepted_at: "2026-10-27T10:00:00Z", reference_result: {image_id: "sha256:proof"}};
  for (const compact of [false, true]) {
    const report = vm.runInContext(`createAttemptReport(attempt, ${compact})`, context);
    assert.equal(report.children.some(child => child.className === "attempt-stages"), false);
    assert.match(report.children.map(child => child.textContent || "").join(" "), /совпал с проверенным оригиналом/);
    assert.match(report.children.map(child => child.textContent || "").join(" "), /сборка и тесты не запускались/);
  }
  context.attempt = {...context.attempt, reference_result: undefined};
  const regular = vm.runInContext("createAttemptReport(attempt)", context);
  assert.equal(regular.children.find(child => child.className === "attempt-stages").children.length, 3);
});

test("hide-and-seek saves edits for multiple files without submitting protected files", () => {
  const answer = {value: "changed", addEventListener() {}};
  const context = browserContext(null, answer);
  const serialized = vm.runInContext(`${source}
    studentDefenseMode = "hide_seek";
    hideEditingPath = "src/first.cpp"; hideEditingAllowed = true; hideEditingOriginal = "original";
    rememberHideEdit();
    hideEditingPath = "include/second.hpp"; hideEditingOriginal = "header";
    document.querySelector("#answer").value = "new header"; rememberHideEdit();
    hideEditingPath = "tests/test.cpp"; hideEditingAllowed = false;
    document.querySelector("#answer").value = "forged";
    defenseAnswer();
  `, context);
  assert.deepEqual(JSON.parse(serialized), {"src/first.cpp": "changed", "include/second.hpp": "new header"});
});

test("restoring the displayed mutated file removes its local override", () => {
  const answer = {value: "mutated", addEventListener() {}};
  const context = browserContext(null, answer);
  const serialized = vm.runInContext(`${source}
    studentDefenseMode = "hide_seek"; hideEdits = {"main.cpp": "changed"};
    hideEditingPath = "main.cpp"; hideEditingAllowed = true; hideEditingOriginal = "mutated";
    defenseAnswer();
  `, context);
  assert.deepEqual(JSON.parse(serialized), {});
});

function browserContext(wheel, answer = null) {
  return vm.createContext({
    alert() {},
    clearTimeout,
    console,
    crypto: { randomUUID: () => "00000000-0000-4000-8000-000000000000" },
    decodeURIComponent,
    document: {
      cookie: "",
      documentElement: { dataset: {} },
      querySelector(selector) {
        if (selector === "#function-wheel") return wheel;
        if (selector === "#answer") return answer;
        return null;
      },
      querySelectorAll() { return []; },
    },
    localStorage: { getItem: () => null, setItem() {} },
    matchMedia: () => ({ matches: false }),
    setTimeout,
    window: { addEventListener() {} },
  });
}

test("wheel renderer accepts a missing candidate list while preparation is pending", () => {
  const wheel = {
    dataset: {},
    querySelectorAll: () => [],
    style: { removeProperty() {} },
  };
  const context = browserContext(wheel);
  vm.runInContext(`${source}\nrenderWheelCandidates(null);`, context);
  assert.equal(wheel.dataset.candidates, "");
});

test("selected wheel sector is rotated to the fixed pointer at the top", () => {
  const context = browserContext(null);
  const rotation = vm.runInContext(`${source}\nselectedWheelRotation(3, 8);`, context);
  assert.equal(rotation, -135);
});

test("Tab inserts four spaces into the answer without moving focus", () => {
  let keydown;
  const answer = {
    value: "return value;",
    selectionStart: 7,
    selectionEnd: 7,
    scrollTop: 120,
    scrollLeft: 8,
    addEventListener(type, listener) {
      if (type === "keydown") keydown = listener;
    },
    setSelectionRange(start, end) {
      this.selectionStart = start;
      this.selectionEnd = end;
    },
  };
  const context = browserContext(null, answer);
  vm.runInContext(source, context);
  let prevented = false;
  keydown({ key: "Tab", altKey: false, ctrlKey: false, metaKey: false, shiftKey: false, preventDefault() { prevented = true; } });
  assert.equal(prevented, true);
  assert.equal(answer.value, "return     value;");
  assert.equal(answer.selectionStart, 11);
  assert.equal(answer.selectionEnd, 11);
  assert.equal(answer.scrollTop, 120);
  assert.equal(answer.scrollLeft, 8);
});

test("Tab and Shift+Tab indent and outdent selected lines", () => {
  const context = browserContext(null);
  const indented = vm.runInContext(`${source}\neditAnswerIndent("one\\ntwo", 0, 7, false);`, context);
  assert.equal(indented.value, "    one\n    two");
  const outdented = vm.runInContext(`editAnswerIndent("    one\\n    two", 0, 15, true);`, context);
  assert.equal(outdented.value, "one\ntwo");
});

test("C++ highlighter marks basic tokens and escapes source HTML", () => {
  const context = browserContext(null);
  const highlighted = vm.runInContext(`${source}\nhighlightCpp("const int value = 42; // <unsafe>");`, context);
  assert.match(highlighted, /syntax-keyword[^>]*>const<\/span>/);
  assert.match(highlighted, /syntax-keyword[^>]*>int<\/span>/);
  assert.match(highlighted, /syntax-number[^>]*>42<\/span>/);
  assert.match(highlighted, /syntax-comment[^>]*>\/\/ &lt;unsafe&gt;<\/span>/);
  assert.doesNotMatch(highlighted, /<unsafe>/);
});

test("C++ highlighter marks repeated occurrences of the selected identifier", () => {
  const context = browserContext(null);
  const highlighted = vm.runInContext(`${source}\nhighlightCpp("int result = value + value;", "value");`, context);
  assert.equal((highlighted.match(/class="syntax-match"/g) || []).length, 2);
  assert.doesNotMatch(highlighted, /syntax-match[^>]*>int</);
});

test("GitHub importer derives a repository from the site group metadata", () => {
  const context = browserContext(null);
  const repository = vm.runInContext(`${source}\nexpectedGitHubRepository({ organization_login: "is-itmo-c-26" }, "student-one", "labwork1");`, context);
  assert.equal(repository, "is-itmo-c-26/labwork1-student-one");
});

test("Local clone importer rejects metadata for another student or mutable ref", () => {
  const context = browserContext(null);
  vm.runInContext(source, context);
  const metadata = {repository_full_name: "org/labwork1-Student", version_label: "deadline_0", commit_sha: "1".repeat(40), archive_sha256: "2".repeat(64)};
  context.metadata = metadata;
  assert.equal(vm.runInContext('validateCloneMetadata(metadata, "org/labwork1-student").commit_sha', context), metadata.commit_sha);
  assert.throws(() => vm.runInContext('validateCloneMetadata(metadata, "org/labwork1-other")', context));
  context.metadata = {...metadata, commit_sha: "main"};
  assert.throws(() => vm.runInContext('validateCloneMetadata(metadata, "org/labwork1-student")', context));
});

test("Student repository highlights only the selected entity and escapes its source", () => {
  const context = browserContext(null);
  let rendered;
  const code = {replaceChildren(...children) { rendered = children; }, querySelector() { return null; }};
  context.document.querySelector = selector => selector === "#source-code code" ? code : null;
  context.document.createElement = () => ({});
  vm.runInContext(`${source}\nactiveChallenge = {file_path: "main.cpp", begin_line: 2, end_line: 3}; renderStudentSource("header\\nint main() {\\n/* <TODO> */\\n}", "main.cpp");`, context);
  assert.equal(rendered[0].className, "source-line");
  assert.match(rendered[1].className, /source-line--restore/);
  assert.match(rendered[2].className, /source-line--restore/);
  assert.doesNotMatch(rendered[2].innerHTML, /<TODO>/);
  assert.equal(rendered[3].className, "source-line");
});
