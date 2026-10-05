import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import vm from "node:vm";

const source = await readFile(new URL("./app.js", import.meta.url), "utf8");

function browserContext(wheel) {
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
        return selector === "#function-wheel" ? wheel : null;
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
