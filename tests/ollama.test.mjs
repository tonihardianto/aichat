import assert from "node:assert/strict";
import test from "node:test";
import { ollamaUrl, toOllamaMessages } from "../src/lib/ollama.ts";

test("ollamaUrl trims trailing slash", () => {
  assert.equal(ollamaUrl("http://host/v1/", "models"), "http://host/v1/models");
});

test("toOllamaMessages emits text and image_url parts", () => {
  const messages = [{ role: "user", content: "Baca ini", image: { dataUrl: "data:image/png;base64,AA", name: "a.png", type: "image/png" } }];

  assert.deepEqual(toOllamaMessages(messages), [{
    role: "user",
    content: [
      { type: "text", text: "Baca ini" },
      { type: "image_url", image_url: { url: "data:image/png;base64,AA" } },
    ],
  }]);
});
