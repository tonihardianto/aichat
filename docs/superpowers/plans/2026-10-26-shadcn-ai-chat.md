# shadcn AI Chat UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Adapt the entire existing chat UI to shadcn AI components while preserving app behavior.

**Architecture:** Reuse local shadcn AI primitives and existing Radix-based UI controls. Keep chat hooks, Ollama integration, types, and persistence unchanged; connect existing state to a polished responsive shell.

**Tech Stack:** React 19, TypeScript, Tailwind CSS, local shadcn-compatible components.

**Spec:** `docs/superpowers/specs/2026-10-26-shadcn-ai-chat-design.md`

## Global Constraints
- Preserve existing Ollama, image, conversation, streaming, retry, stop, regenerate, theme and accessibility behavior.
- No new runtime dependencies.
- Indonesian UI copy; mobile sidebar remains usable.
- Use local shadcn AI components from `src/components/ui/`.

## Review Focus
- Empty conversation: starter suggestions must send selected text through the normal send flow.
- Streaming response: stop control remains available and message scroll respects user position.
- Image-only prompt: attachment must keep send enabled and render as before.
- Model fetch/demo/error states: model control stays understandable and reload remains available.
- Mobile viewport: history navigation and composer remain reachable without horizontal overflow.

---

### Task 1: AI conversation and empty states

**Files:**
- Modify: `src/App.tsx`
- Modify: `src/components/chat/MessageList.tsx`
- Test: `tests/ui-contract.test.mjs`

**Interfaces:**
- Consumes existing `MessageList` props and `chat.send` callback.
- Produces prompt suggestion controls wired to the normal send flow, with an optional `onSuggestion` callback.

- [ ] **Step 1: Write failing UI contract tests** for semantic AI message component usage, suggestions in empty state, and no empty state when messages exist.
- [ ] **Step 2: Run** `node --test tests/ui-contract.test.mjs`; expect failures for missing suggestions/callback.
- [ ] **Step 3: Implement** suggestion chips and AI empty state; wire each suggestion to `chat.send(text)` in `App.tsx`.
- [ ] **Step 4: Run** `node --test tests/ui-contract.test.mjs`; expect PASS.

### Task 2: Composer and model controls

**Files:**
- Modify: `src/components/chat/Composer.tsx`
- Modify: `src/components/chat/ModelSelect.tsx`
- Test: `tests/ui-contract.test.mjs`

**Interfaces:**
- Preserve existing component props and upload/send behavior.

- [ ] **Step 1: Add failing assertions** for shadcn AI input-group primitives, attachment affordance, and model selector loading/error/demo labels.
- [ ] **Step 2: Run** `node --test tests/ui-contract.test.mjs`; expect new assertions to fail.
- [ ] **Step 3: Adapt** composer/model selector styling and accessible labels using existing local controls; do not change image validation or send logic.
- [ ] **Step 4: Run** `node --test tests/ui-contract.test.mjs`; expect PASS.

### Task 3: Application shell, history, and theme

**Files:**
- Modify: `src/App.tsx`
- Modify: `src/components/chat/Sidebar.tsx`
- Modify: `src/components/chat/MessageBubble.tsx`
- Modify: `src/index.css`
- Test: `tests/ui-contract.test.mjs`

**Interfaces:**
- Preserve existing sidebar callbacks, model props, theme preference, and message actions.

- [ ] **Step 1: Add failing assertions** for shell/sidebar shadcn structure and mobile history entry point.
- [ ] **Step 2: Run** `node --test tests/ui-contract.test.mjs`; expect new assertions to fail.
- [ ] **Step 3: Adapt** app chrome and sidebar to consistent shadcn semantic tokens; retain actions and responsive Sheet.
- [ ] **Step 4: Run** `node --test tests/ui-contract.test.mjs`; expect PASS.

### Task 4: Full verification

**Files:**
- Test: `tests/ui-contract.test.mjs`
- Existing tests: `tests/*.test.mjs`

- [ ] **Step 1: Run** `npm run build`; expect TypeScript and production build success.
- [ ] **Step 2: Run** `node --test tests/*.test.mjs`; expect all tests pass.
- [ ] **Step 3: Inspect** `git diff --check`; expect no whitespace errors.
