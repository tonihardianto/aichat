# Ollama Model dan Gambar Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Connect chat to selectable Ollama models and send optional image attachments using OpenAI-compatible content parts.

**Architecture:** Extend the existing agent adapter with model discovery and OpenAI chat-completions payload conversion. A small `useModels` hook owns remote model state and local selected-model persistence. Composer reads one validated image into a data URL; messages retain attachment metadata so history and regeneration preserve image inputs.

**Tech Stack:** React, TypeScript, Vite, shadcn/ui, browser FileReader/fetch, Node built-in test runner.

**Spec:** `docs/superpowers/specs/2026-10-26-ollama-model-images-design.md`

## Global Constraints

- Use `VITE_OLLAMA_BASE_URL`; e.g. `http://10.20.30.35:11434/v1`.
- Get model IDs from `${baseUrl}/models`; stream requests go to `${baseUrl}/chat/completions`.
- Send OpenAI-compatible `{ model, messages, stream: true }`.
- Only one JPEG, PNG, or WebP image, maximum 5 MB, per user message.
- Store selected model and image data URL in browser localStorage with chat data.
- Empty base URL keeps demo chat available.
- All user-facing copy is Indonesian.
- Do not add dependencies.

---

## File Structure

```text
src/types/chat.ts                 add image attachment to Message
src/lib/ollama.ts                 URL/payload pure helpers
src/lib/agent.ts                  listModels + selected-model stream request
src/hooks/useModels.ts            fetch/reload/list/persist selected model
src/hooks/useChat.ts              accept attachment in send/regenerate history
src/components/chat/ModelSelect.tsx model selector, load/retry status
src/components/chat/Composer.tsx  file input, preview, validation, submit attachment
src/App.tsx                       compose model and attachment state
tests/ollama.test.mjs             helper contract tests
docs/README.md                    Ollama environment/configuration docs
```

### Task 1: Add attachment and Ollama payload contracts

**Files:**
- Modify: `src/types/chat.ts`, `src/lib/agent.ts`
- Create: `src/lib/ollama.ts`, `tests/ollama.test.mjs`

**Interfaces:**
- Produces: `ImageAttachment`, `ollamaUrl(baseUrl, path)`, `toOllamaMessages(messages)`, `listModels()`, and `streamReply(messages, model, signal, onToken)`.

- [ ] **Step 1: Write failing helper tests**

```js
test("ollamaUrl trims trailing slash", () => {
  assert.equal(ollamaUrl("http://host/v1/", "models"), "http://host/v1/models");
});

test("toOllamaMessages emits text and image_url parts", () => {
  const messages = [{ role: "user", content: "Baca ini", image: { dataUrl: "data:image/png;base64,AA", name: "a.png", type: "image/png" } }];
  assert.deepEqual(toOllamaMessages(messages), [{ role: "user", content: [{ type: "text", text: "Baca ini" }, { type: "image_url", image_url: { url: "data:image/png;base64,AA" } }] }]);
});
```

- [ ] **Step 2: Run test red**

Run: `node --test tests/ollama.test.mjs`

Expected: fail because `src/lib/ollama.ts` is missing.

- [ ] **Step 3: Implement types and pure helpers**

```ts
export type ImageAttachment = { name: string; type: "image/jpeg" | "image/png" | "image/webp"; dataUrl: string };
export type Message = { id: string; role: Role; content: string; image?: ImageAttachment; createdAt: string };
```

`toOllamaMessages` emits string content for text-only messages and content parts for attached images. `ollamaUrl` removes trailing `/` and appends one path separator.

- [ ] **Step 4: Implement adapter model APIs**

```ts
export async function listModels(): Promise<string[]>;
export async function streamReply(messages: Message[], model: string | null, signal: AbortSignal, onToken: (token: string) => void): Promise<void>;
```

`listModels` returns `data[].id`, sorts names, and throws Indonesian error on HTTP/network/invalid response. `streamReply` uses demo when base URL missing; otherwise requires model and POSTs `${baseUrl}/chat/completions` body `{ model, messages: toOllamaMessages(messages), stream: true }`. Reuse existing text/SSE parser.

- [ ] **Step 5: Run test green and build**

Run: `node --test tests/ollama.test.mjs && npm run build`

Expected: all tests pass and production build succeeds.

- [ ] **Step 6: Commit**

```bash
git add src/types/chat.ts src/lib/ollama.ts src/lib/agent.ts tests/ollama.test.mjs
git commit -m "feat: add Ollama model transport"
```

### Task 2: Persist and select available models

**Files:**
- Create: `src/hooks/useModels.ts`, `src/components/chat/ModelSelect.tsx`
- Modify: `src/App.tsx`

**Interfaces:**
- Consumes: `listModels(): Promise<string[]>`.
- Produces: `useModels()` result `{ models, selectedModel, setSelectedModel, isLoading, error, reload, isDemo }`.

- [ ] **Step 1: Implement model hook**

```ts
const MODEL_KEY = "ai-chat-model";
export function useModels() {
  // fetch once if VITE_OLLAMA_BASE_URL exists; persist selected ID; choose first loaded model if stored ID missing
}
```

On model fetch failure expose Indonesian error and retain text chat. Empty base URL returns `isDemo: true`, no request, and no error. `reload` retries fetch.

- [ ] **Step 2: Implement selector component**

```tsx
type ModelSelectProps = { models: string[]; value: string | null; loading: boolean; error: string | null; isDemo: boolean; onChange: (model: string) => void; onReload: () => void; };
```

Render labelled native select for small dependency-free control, reload icon button with `aria-label="Muat ulang model"`, loading state, `Mode demo` status, and a short inline error with reload action.

- [ ] **Step 3: Compose App**

```tsx
const models = useModels();
const chat = useChat(conversations.activeConversation, conversations.updateConversation, models.selectedModel);
```

Place `<ModelSelect>` in header without breaking 360px layout; header wraps controls on narrow screens.

- [ ] **Step 4: Verify**

Run: `node --test tests/ollama.test.mjs && npm run build`

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add src/hooks/useModels.ts src/components/chat/ModelSelect.tsx src/App.tsx
git commit -m "feat: add Ollama model selector"
```

### Task 3: Add image upload and chat attachment flow

**Files:**
- Modify: `src/hooks/useChat.ts`, `src/components/chat/Composer.tsx`, `src/components/chat/MessageBubble.tsx`, `src/App.tsx`

**Interfaces:**
- Consumes: `ImageAttachment` and `useChat.send(content, image?)`.
- Produces: validated one-image composer and persisted user message attachment.

- [ ] **Step 1: Update chat send contract**

```ts
send(content: string, image?: ImageAttachment): Promise<void>
```

Permit image-only messages. Include image when constructing user message; preserve it through retry and regeneration. Assistant messages never have images.

- [ ] **Step 2: Add composer image picker**

```tsx
<input ref={inputRef} className="sr-only" type="file" accept="image/jpeg,image/png,image/webp" onChange={readImage} />
```

Use FileReader `readAsDataURL`; reject non-JPEG/PNG/WebP or over `5 * 1024 * 1024` bytes with Indonesian message. Display image preview, name, remove icon `aria-label="Hapus gambar"`. Disable send only when both text and image absent. Clear attachment after successful submission.

- [ ] **Step 3: Render persisted user attachment**

```tsx
{message.image && <img src={message.image.dataUrl} alt={`Lampiran ${message.image.name}`} className="mb-2 max-h-72 rounded-md object-contain" />}
```

Show image above user text; never render attachments as assistant content.

- [ ] **Step 4: Wire App and verify**

Pass accepted attachment through Composer to chat hook. Run:

```bash
node --test tests/ollama.test.mjs && npm run build
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add src/hooks/useChat.ts src/components/chat/Composer.tsx src/components/chat/MessageBubble.tsx src/App.tsx
git commit -m "feat: send image attachments to Ollama"
```

### Task 4: Document Ollama configuration

**Files:**
- Modify: `docs/README.md`

**Interfaces:**
- Consumes: `VITE_OLLAMA_BASE_URL` and adapter payload contract.
- Produces: exact Ollama setup instructions.

- [ ] **Step 1: Document configuration**

```env
VITE_OLLAMA_BASE_URL=http://10.20.30.35:11434/v1
```

State model selector loads `${VITE_OLLAMA_BASE_URL}/models`, chat uses `/chat/completions`, and images require an installed vision-capable model. Document accepted image types and 5 MB limit.

- [ ] **Step 2: Verify**

Run: `node --test tests/ollama.test.mjs && npm run build`

Expected: all pass.

- [ ] **Step 3: Commit**

```bash
git add docs/README.md
git commit -m "docs: explain Ollama model and image setup"
```

## Self-review

- Spec coverage: Task 1 covers endpoint/payload/type/storage contract; Task 2 covers discovery, selector, persistence/reload/demo; Task 3 covers valid image handling, preview, persistence, message conversion; Task 4 covers setup docs.
- Placeholder scan: none.
- Type consistency: `ImageAttachment`, `Message.image`, `streamReply(messages, model, signal, onToken)`, `useChat.send(content, image?)`, and `useModels.selectedModel` are declared before use.
