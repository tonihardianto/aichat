import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const source = (path) => readFileSync(new URL(path, import.meta.url), "utf8");

test("empty chat offers prompt suggestions through AI UI", () => {
  const app = source("../src/App.tsx");
  const list = source("../src/components/chat/MessageList.tsx");
  assert.match(list, /Suggestion/);
  assert.match(app, /onSuggestion/);
  assert.match(app, /chat\.send/);
});

test("chat view uses shadcn AI message primitives", () => {
  const list = source("../src/components/chat/MessageList.tsx");
  const bubble = source("../src/components/chat/MessageBubble.tsx");
  const messageUi = source("../src/components/ui/message.tsx");
  const bubbleUi = source("../src/components/ui/bubble.tsx");
  assert.match(list, /MessageGroup/);
  assert.match(bubble, /@\/components\/ui\/message/);
  assert.match(bubble, /@\/components\/ui\/bubble/);
  assert.match(bubble, /Bubble align="start" variant="ghost"/);
  assert.match(bubble, /BubbleContent[\s\S]*markdown-content/);
  assert.match(messageUi, /MessageAvatar/);
  assert.match(messageUi, /data-slot="message-group"/);
  assert.match(messageUi, /data-slot="message-footer"/);
  assert.match(bubbleUi, /BubbleReactions/);
  assert.match(bubbleUi, /data-slot="bubble-content"/);
  assert.match(bubbleUi, /max-w-\[80%\]/);
  assert.match(bubbleUi, /data-variant={variant}/);
});

test("composer AI input-group follows shadcn structure", () => {
  const inputGroup = source("../src/components/ui/input-group.tsx");
  assert.match(inputGroup, /data-slot="input-group"/);
  assert.match(inputGroup, /role="group"/);
  assert.match(inputGroup, /has-\[\[data-slot=input-group-control\]:focus-visible\]/);
  assert.match(inputGroup, /size = "icon-sm"/);
  assert.doesNotMatch(inputGroup, /variant = "ghost", size = "icon"/);
  const composer = source("../src/components/chat/Composer.tsx");
  assert.match(composer, /size="icon-sm"[\s\S]*?variant="outline"[\s\S]*?aria-label="Lampirkan gambar"/);
  assert.match(composer, /size="icon-sm"[\s\S]*?variant="default"[\s\S]*?aria-label="Kirim"/);
  assert.match(composer, /rounded-full/);
});

test("composer textarea does not stretch vertically when addon stacks below", () => {
  const inputGroup = source("../src/components/ui/input-group.tsx");
  const textarea = inputGroup.match(/const InputGroupTextarea[\s\S]*?\n}\)/)?.[0] ?? "";
  assert.match(textarea, /min-h-12/);
  assert.doesNotMatch(textarea, /flex-1/);
});

test("composer keeps input-group and attachment controls", () => {
  const composer = source("../src/components/chat/Composer.tsx");
  const attachment = source("../src/components/ui/attachment.tsx");
  assert.match(composer, /InputGroupTextarea/);
  assert.match(composer, /PlusIcon/);
  assert.match(composer, /Attachment/);
  assert.match(attachment, /data-slot="attachment"/);
  assert.match(composer, /image\/jpeg,image\/png,image\/webp/);
});

test("sidebar and app shell retain responsive shadcn navigation", () => {
  const app = source("../src/App.tsx");
  const sidebar = source("../src/components/chat/Sidebar.tsx");
  assert.match(app, /SheetContent/);
  assert.match(sidebar, /DropdownMenu/);
  assert.match(sidebar, /AlertDialog/);
  assert.match(sidebar, /Ollama lokal/);
  assert.match(app, /ModelSelect/);
  assert.match(app, /ai-chat-theme/);
});

test("model selector uses shadcn controls and exposes model label", () => {
  const models = source("../src/components/chat/ModelSelect.tsx");
  assert.match(models, /Model Ollama/);
  assert.match(models, /<Button/);
  assert.match(models, /bg-secondary/);
  assert.match(models, /role="alert"/);
});

test("assistant message actions stay available for keyboard users", () => {
  const bubble = source("../src/components/chat/MessageBubble.tsx");
  assert.match(bubble, /Buat ulang jawaban/);
  assert.match(bubble, /Salin/);
  assert.match(bubble, /group-focus-within:opacity-100/);
});

test("app shell and message bubble provide global TooltipProvider context", () => {
  const app = source("../src/App.tsx");
  const bubble = source("../src/components/chat/MessageBubble.tsx");
  assert.match(app, /TooltipProvider/);
  assert.match(bubble, /TooltipProvider/);
});

test("theme and chat use semantic shadcn tokens", () => {
  const app = source("../src/App.tsx");
  const sidebar = source("../src/components/chat/Sidebar.tsx");
  assert.match(app, /bg-primary text-primary-foreground/);
  assert.match(sidebar, /bg-card/);
  assert.doesNotMatch(app, /bg-blue-600/);
});

 test("composer preserves image-only submit and upload limits", () => {
  const composer = source("../src/components/chat/Composer.tsx");
  assert.match(composer, /!value\.trim\(\) && !image/);
  assert.match(composer, /file\.size > 5 \* 1024 \* 1024/);
  assert.match(composer, /disabled=\{!value\.trim\(\) && !image\}/);
});

test("composer supports pasting images from clipboard", () => {
  const composer = source("../src/components/chat/Composer.tsx");
  assert.match(composer, /onPaste/);
  assert.match(composer, /clipboardData/);
});

 test("message history retains near-bottom scrolling behavior", () => {
  const list = source("../src/components/chat/MessageList.tsx");
  assert.match(list, /nearBottom\.current/);
  assert.match(list, /scrollHeight/);
});

test("conversation messages and composer share a compact responsive width", () => {
  const list = source("../src/components/chat/MessageList.tsx");
  const composer = source("../src/components/chat/Composer.tsx");
  assert.match(list, /max-w-\[920px\]/);
  assert.match(list, /gap-4/);
  assert.match(list, /py-5/);
  assert.match(composer, /max-w-\[920px\]/);
});

test("assistant message group uses compact readable width", () => {
  const list = source("../src/components/chat/MessageList.tsx");
  assert.match(list, /max-w-\[920px\]/);
  assert.match(list, /gap-4/);
  assert.match(list, /py-5/);
});

 test("mobile app shell has no forced horizontal minimum width", () => {
  const app = source("../src/App.tsx");
  assert.doesNotMatch(app, /min-w-\[360px\]/);
});

test("assistant HTML and JSX demos render only inside a sandboxed preview", () => {
  const bubble = source("../src/components/chat/MessageBubble.tsx");
  const preview = source("../src/components/chat/CodePreview.tsx");
  assert.match(bubble, /CodePreview/);
  assert.match(preview, /sandbox="allow-scripts"/);
  assert.match(preview, /default-src 'none'/);
  assert.match(preview, /srcDoc/);
  assert.match(preview, /htmlDocument/);
  assert.match(preview, /language === "jsx"/);
});

test("copying highlighted code preserves token text", () => {
  const bubble = source("../src/components/chat/MessageBubble.tsx");
  assert.match(bubble, /function nodeText/);
  assert.match(bubble, /nodeText\(node as ElementContent\)/);
  assert.doesNotMatch(bubble, /children\.join\(""\)/);
});

test("syntax highlight selectors survive Tailwind build", () => {
  const css = source("../src/index.css");
  const highlight = css.slice(css.indexOf(".hljs"));
  assert.match(highlight, /\.hljs-keyword/);
  assert.match(highlight, /\.hljs-string/);
  assert.ok(css.lastIndexOf(".hljs-keyword") > css.lastIndexOf("@layer base"));
});
