import type { Message } from "@/types/chat";

const MAX_TITLE = 40;

/** Builds a conversation title from the first user message. */
export function titleFromMessages(messages: Message[]): string | null {
  const first = messages.find((m) => m.role === "user");
  if (!first) return null;
  const text = first.content
    .replace(/```[\s\S]*?```/g, " ")
    .replace(/[#*_`>~\[\]()]/g, "")
    .replace(/\s+/g, " ")
    .trim();
  if (!text) return first.image ? "Gambar" : null;
  if ([...text].length <= MAX_TITLE) return text;
  const cut = [...text].slice(0, MAX_TITLE).join("");
  const lastSpace = cut.lastIndexOf(" ");
  return (lastSpace > MAX_TITLE / 2 ? cut.slice(0, lastSpace) : cut).trimEnd() + "…";
}
