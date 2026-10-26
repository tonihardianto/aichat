import assert from "node:assert/strict";
import test from "node:test";
import { createServer } from "vite";
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

test("tokenFromValue reads OpenAI chat completion SSE delta content", async () => {
  const server = await createServer({ configFile: "vite.config.ts", server: { middlewareMode: true } });
  try {
    const { tokenFromValue } = await server.ssrLoadModule("/src/lib/agent.ts");
    assert.equal(tokenFromValue({ choices: [{ delta: { content: "token" } }] }), "token");
  } finally {
    await server.close();
  }
});
