import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import vm from "node:vm";

const source = await readFile(new URL("./app.js", import.meta.url), "utf8");

function defenseBrowser() {
  const elements = new Map();
  const element = () => ({value: "", textContent: "", dataset: {}, style: {}, children: [],
    addEventListener() {}, setAttribute(name, value) { this[name] = value; }, replaceChildren(...children) { this.children = children; },
    classList: {add() {}, remove() {}, toggle() {}}, querySelectorAll() { return []; }});
  const answer = element();
  elements.set("#answer", answer);
  for (const selector of ["#defense-status", "#timer", "#defense-timer-bar", "#save-draft", "#submit-attempt", "#answer-signature", "#answer-closing", "#source-access-hint", "#repository-files", "#source-code code", "#source-title", "#restore-tabs"]) elements.set(selector, element());
  const context = browserContext(null, answer);
  context.document.querySelector = selector => elements.get(selector) || null;
  context.document.createElement = () => element();
  vm.runInContext(source, context);
  vm.runInContext(`
    defenseID = "defense";
    renderRepositoryTree = (root, items) => { root.items = items; };
    renderStudentSource = (content, path) => { document.querySelector("#source-code code").textContent = content; };
  `, context);
  return {context, elements};
}

test("teacher can select a mixed set and toggle an element without losing the rest", () => {
  const {context, elements} = defenseBrowser();
  const form = {elements: {hidden_count: {value: "3"}, candidate_id: {value: ""}}};
  elements.set("#configure-defense-form", form);
  elements.set("#selected-candidate", {textContent: ""});
  context.candidates = ["struct", "function", "function", "class"].map((type, index) => ({id: String(index), entity_type: type, signature: `${type} Item${index}`, file_path: "main.cpp", begin_line: index, end_line: index + 1}));
  vm.runInContext(`teacherDefense = {candidate_catalog: candidates}; candidates.slice(0,3).forEach(selectTeacherCandidate);`, context);
  assert.equal(vm.runInContext("teacherSelectedIDs.join(',')", context), "0,1,2");
  vm.runInContext("selectTeacherCandidate(candidates[3])", context);
  assert.equal(vm.runInContext("teacherSelectedIDs.length", context), 3);
  vm.runInContext("selectTeacherCandidate(candidates[1])", context);
  assert.equal(vm.runInContext("teacherSelectedIDs.join(',')", context), "0,2");
  assert.match(elements.get("#selected-candidate").textContent, /func_1.*struct/);
});

test("multi-restore tabs retain separate drafts and hidden signatures never appear in boundaries", async () => {
  const {context, elements} = defenseBrowser();
  const tasks = [{id: "one", label: "func_1", signature: "", function_name: "func_1", entity_type: "struct", file_path: "main.cpp", begin_line: 1, end_line: 1}, {id: "two", label: "func_2", signature: "", function_name: "func_2", entity_type: "function", file_path: "main.cpp", begin_line: 2, end_line: 2}];
  context.value = {id: "defense", status: "active", defense_mode: "restore", signatures_visible: false, current_draft: JSON.stringify({one: "struct Box {int x;};", two: "int f(){return 1;}"}), challenge: tasks[0], challenges: tasks};
  context.api = async url => url.endsWith("/repository") ? {items: [{path: "main.cpp"}]} : url.includes("/repository/file") ? {path: "main.cpp", content: "/* func_1 */\n/* func_2 */", masked: true} : context.value;
  await vm.runInContext("refreshDefense()", context);
  assert.equal(elements.get("#restore-tabs").children.length, 2);
  assert.equal(elements.get("#answer").value, "struct Box {int x;};");
  assert.equal(elements.get("#answer-signature").textContent, "");
  assert.equal(elements.get("#answer-closing").textContent, "");
  elements.get("#answer").value = "struct Box {int y;};";
  await vm.runInContext('selectRestoreTab("two")', context);
  assert.equal(elements.get("#answer").value, "int f(){return 1;}");
  elements.get("#answer").value = "int f(){return 2;}";
  await vm.runInContext("refreshDefense()", context);
  await vm.runInContext('selectRestoreTab("one")', context);
  assert.equal(elements.get("#answer").value, "struct Box {int y;};");
  assert.deepEqual(JSON.parse(vm.runInContext("defenseAnswer()", context)), {one: "struct Box {int y;};", two: "int f(){return 2;}"});
  assert.equal(elements.get("#submit-attempt").disabled, false);
});

test("student loads selected signature and source on ready-to-active transition", async () => {
  const {context, elements} = defenseBrowser();
  context.value = {status: "ready", defense_mode: "restore", draft_version: 1};
  context.api = async url => {
    if (url.endsWith("/repository")) return {items: [{path: "main.cpp"}]};
    if (url.includes("/repository/file")) return {path: "main.cpp", content: "int sum() {/* TODO */}", masked: true};
    return context.value;
  };
  await vm.runInContext("refreshDefense()", context);
  assert.equal(elements.get("#submit-attempt").disabled, true);
  context.value = {...context.value, status: "active", deadline_at: new Date(Date.now() + 60000).toISOString(), selected_candidate_id: "candidate", challenge: {signature: "int sum()", function_name: "sum", file_path: "main.cpp", begin_line: 1, end_line: 1}};
  await vm.runInContext("refreshDefense()", context);
  assert.equal(elements.get("#answer-signature").textContent, "int sum() {");
  assert.match(elements.get("#source-code code").textContent, /TODO/);
  assert.equal(elements.get("#repository-files").items[0].path, "main.cpp");
  assert.equal(elements.get("#submit-attempt").disabled, false);
});

test("a failed initial file request is retried and does not freeze task loading", async () => {
  const {context, elements} = defenseBrowser();
  let attempts = 0;
  context.api = async url => {
    if (url.endsWith("/repository")) return {items: [{path: "main.cpp"}]};
    if (url.includes("/repository/file")) {
      if (++attempts === 1) throw new Error("temporary file failure");
      return {path: "main.cpp", content: "masked", masked: true};
    }
    return {status: "active", defense_mode: "restore", deadline_at: new Date(Date.now() + 60000).toISOString(), challenge: {signature: "int sum()", file_path: "main.cpp"}};
  };
  await assert.rejects(vm.runInContext("refreshDefense()", context), /temporary/);
  assert.equal(vm.runInContext("repositoryLoaded", context), false);
  assert.equal(elements.get("#submit-attempt").disabled, true);
  assert.match(elements.get("#timer").textContent, /^\d+:\d{2}$/);
  await vm.runInContext("refreshDefense()", context);
  assert.equal(attempts, 2);
  assert.equal(vm.runInContext("repositoryLoaded", context), true);
  assert.equal(elements.get("#submit-attempt").disabled, false);
});

test("an active defense without a challenge explains the error and cannot submit blindly", async () => {
  const {context, elements} = defenseBrowser();
  context.api = async () => ({status: "active", defense_mode: "restore", preparation_error: "Выберите другую версию"});
  await vm.runInContext("refreshDefense()", context);
  assert.equal(elements.get("#submit-attempt").disabled, true);
  assert.equal(elements.get("#answer").readOnly, true);
  assert.equal(elements.get("#defense-status").textContent, "Выберите другую версию");
});

test("viewing a read-only test file in hide mode does not block submission of saved source edits", async () => {
  const {context, elements} = defenseBrowser();
  context.api = async url => {
    if (url.endsWith("/repository")) return {items: [{path: "tests.cpp", editable: false}]};
    if (url.includes("/repository/file")) return {path: "tests.cpp", content: "read only", editable: false};
    return {status: "active", defense_mode: "hide_seek", error_count: 1};
  };
  await vm.runInContext("refreshDefense()", context);
  assert.equal(elements.get("#answer").readOnly, true);
  assert.equal(elements.get("#submit-attempt").disabled, false);
});

test("wheel animation failure still displays selected function", () => {
  const wheel = {dataset: {}, style: {}, classList: {add() {}}, animate() { throw new Error("animation unavailable"); }};
  const context = browserContext(wheel);
  const label = {};
  context.getComputedStyle = () => ({transform: "none"});
  const select = context.document.querySelector;
  context.document.querySelector = selector => selector === "#wheel-label" ? label : select(selector);
  vm.runInContext(source + `\nwheelCandidates = [{id: "one", signature: "int sum()", file_path: "main.cpp"}]; settleWheel("one", {signature: "int sum()", function_name: "sum", file_path: "main.cpp"});`, context);
  assert.equal(label.textContent, "sum");
  assert.equal(wheel.style.transform, "rotate(0deg)");
});

test("timings show measured stages and distinguish missing measurements", () => {
  const context = browserContext(null);
  context.document.createElement = tag => ({tag, children: [], append(...children) {this.children.push(...children);}});
  vm.runInContext(source, context);
  const report = vm.runInContext("createTimingReport({queue_ms: 12, compile_ms: null, snapshot_hit: true})", context);
  const text = item => [item.textContent || "", ...(item.children || []).map(text)].join(" ");
  assert.match(text(report), /12\.0 мс/);
  assert.match(text(report), /Нет замера/);
  assert.match(text(report), /отдельной рабочей копии/);
  assert.equal(vm.runInContext("createTimingReport(null)", context), null);
});

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
