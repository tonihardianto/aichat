export type Role = "user" | "assistant";

export type ImageAttachment = {
  name: string;
  type: "image/jpeg" | "image/png" | "image/webp";
  dataUrl: string;
};

export type Message = {
  id: string;
  role: Role;
  content: string;
  image?: ImageAttachment;
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
