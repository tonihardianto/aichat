# Task 3 report: image upload and chat attachment flow

## Changes

- `src/hooks/useChat.ts`: `send(content, image?)` returns `Promise<void>`, accepts image-only messages, and stores image with user message. Retry and regeneration reuse original message list, preserving attachment.
- `src/components/chat/Composer.tsx`: one-image picker, MIME and exact 5 MiB validation, FileReader data URL preview, filename, remove control, Indonesian validation/read errors, and image-only send. Draft clears only after successful send.
- `src/components/chat/MessageBubble.tsx`: renders persisted user image above text; assistant attachment never rendered.
- `src/App.tsx`: no change needed; existing `chat.send` prop wiring now passes optional attachment through Composer.

## Tests and build

- `node --test tests/ollama.test.mjs`: passed, 2 tests.
- `npm run build`: passed. Vite reports existing-style bundle size warning: main chunk exceeds 500 kB.

## Self-review

- No new dependencies.
- Retry/regenerate preserve attachment through existing `Message[]` flow.
- Validation allows files exactly `5 * 1024 * 1024` bytes and rejects larger files.
- Assistant-only rendering guard prevents assistant images appearing as content.

## Concerns

- UI upload interactions lack dedicated automated tests; requested Ollama tests and production build pass.
- Existing untracked files `ChatPage.tsx`, `ollama-chat.html`, and `readme.md` left untouched.

## Commit

- `94feffd feat: send image attachments to Ollama`
