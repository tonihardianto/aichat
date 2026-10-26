import type { ImageAttachment, Role } from "../types/chat.ts";

type MessageInput = {
  role: Role;
  content: string;
  image?: ImageAttachment;
};

type OllamaContentPart =
  | { type: "text"; text: string }
  | { type: "image_url"; image_url: { url: string } };

export type OllamaMessage = {
  role: Role;
  content: string | OllamaContentPart[];
};

export function ollamaUrl(baseUrl: string, path: string): string {
  return `${baseUrl.replace(/\/+$/, "")}/${path.replace(/^\/+/, "")}`;
}

export function toOllamaMessages(messages: MessageInput[]): OllamaMessage[] {
  return messages.map(({ role, content, image }) => ({
    role,
    content: image
      ? [
          { type: "text", text: content },
          { type: "image_url", image_url: { url: image.dataUrl } },
        ]
      : content,
  }));
}
