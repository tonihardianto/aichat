# Task 2 report

## Changes

- Added `useModels`: loads Ollama models once, persists valid model under `ai-chat-model`, defaults to first available model, exposes reload/loading/error/demo state.
- Added compact native `ModelSelect`: labelled select, loading state, demo status, reload control, inline Indonesian error/reload action.
- Header wraps controls at narrow widths. `useChat` now passes selected model to `streamReply`.

## Tests and build

- `node --test tests/ollama.test.mjs` — 2 pass, 0 fail.
- `npm run build` — pass. Vite warns bundled JS exceeds 500 kB.

## Files

- `src/hooks/useModels.ts`
- `src/components/chat/ModelSelect.tsx`
- `src/App.tsx`
- `src/hooks/useChat.ts`

## Self-review

- No request runs with blank Ollama model after model list loads.
- Demo base URL path makes no model-list request and keeps demo status.
- Stored model falls back to first loaded model when no longer available.
- No image UI changed.

## Concerns

- Existing test suite covers Ollama URL/message conversion only; no React hook/component runner is installed.
- Existing Vite chunk-size warning remains.
