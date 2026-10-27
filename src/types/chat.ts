export type Role = "user" | "assistant";

export type ImageAttachment = {
  id?: string;
  name: string;
  type: "image/jpeg" | "image/png" | "image/webp";
  dataUrl: string;
};

export type Message = {
  id: string;
  role: Role;
  content: string;
  status?: "complete" | "streaming" | "cancelled" | "failed";
  image?: ImageAttachment;
  outputTokens?: number;
  tokensPerSecond?: number;
  createdAt: string;
};

export type Conversation = {
  id: string;
  title: string;
  messages: Message[];
  createdAt: string;
  updatedAt: string;
};

export const UNTITLED_CONVERSATION = "Percakapan baru";
