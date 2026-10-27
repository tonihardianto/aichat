# shadcn AI Chat UI — Design

## Intent
Use shadcn UI and its AI-oriented components throughout existing Ollama chat app. Preserve functional behavior and data flow; replace/adapt visual interface, not provider integration.

## Goals
- Apply AI chat primitives to conversation messages, composer, attachments, streaming indicator, prompt suggestions, model selector, error/empty states, history/sidebar, and responsive navigation.
- Preserve Ollama model discovery, chat streaming, stop/regenerate/retry, image upload/rendering, conversation persistence and management, theme preference, accessibility, and Indonesian UI copy.
- Reuse installed components and semantic theme tokens; avoid dependency additions unless needed for available shadcn UI.

## Non-goals
- No changes to Ollama API behavior or message data model unless required to connect UI.
- No new capabilities such as tools, auth, file upload beyond current image support, or settings persistence redesign.

## Visual direction
Quiet, focused, light/dark semantic palette already in `src/index.css`; clear chat hierarchy with ample readable message width. Sidebar holds conversation history and new-chat action. Header holds app identity, model selection, and theme. Empty state offers useful prompt suggestions; composer remains anchored with attachment and stop/send controls. Responsive sidebar uses existing Sheet behavior.

## Components
Use existing local shadcn-compatible AI components (`bubble`, `message`, `input-group`) as foundations. Use existing button, dropdown, dialog and sheet UI controls. Adapt components and layouts to real conversation/model state. Do not replace working local implementations with registry items whose shadcn registry installation endpoint is unavailable.

## Acceptance
- Entire app UI, including sidebar and mobile navigation, follows the same shadcn visual language.
- All existing interactions remain functional.
- No new runtime dependencies.
- `npm run build` succeeds and `node --test tests/*.test.mjs` passes.
