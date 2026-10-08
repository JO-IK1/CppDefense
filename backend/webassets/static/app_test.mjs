import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import vm from "node:vm";

const source = await readFile(new URL("./app.js", import.meta.url), "utf8");

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
