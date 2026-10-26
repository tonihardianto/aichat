import assert from "node:assert/strict";
import test from "node:test";
import { resolveTheme } from "../src/lib/theme.ts";

test("resolveTheme memakai pilihan eksplisit atau preferensi sistem", () => {
  assert.equal(resolveTheme("light", true), "light");
  assert.equal(resolveTheme("dark", false), "dark");
  assert.equal(resolveTheme("system", true), "dark");
  assert.equal(resolveTheme("system", false), "light");
});
