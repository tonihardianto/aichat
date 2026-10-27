import type { Role } from "@/types/chat";

export type AuthUser = {
  id: string;
  email: string;
  createdAt: string;
  updatedAt: string;
};

export type ApiAttachment = {
  id: string;
  userId?: string;
  objectKey?: string;
  name: string;
  contentType: string;
  byteSize: number;
  sha256?: string;
  createdAt?: string;
};

export type ApiMessage = {
  id: string;
  conversationId?: string;
  role: Role;
  content: string;
  status: "complete" | "streaming" | "cancelled" | "failed";
  model?: string;
  inputTokens?: number;
  outputTokens?: number;
  tokensPerSecond?: number;
  createdAt: string;
  completedAt?: string;
  attachments?: ApiAttachment[];
};

export type ApiConversation = {
  id: string;
  userId?: string;
  title: string;
  createdAt: string;
  updatedAt: string;
};

export type ApiConversationDetail = ApiConversation & {
  messages: ApiMessage[];
};

export class ApiError extends Error {
  code: string;
  status: number;
  constructor(code: string, message: string, status: number) {
    super(message);
    this.code = code;
    this.status = status;
    this.name = "ApiError";
  }
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers || {});
  if (!headers.has("Content-Type") && !(options.body instanceof FormData)) {
    headers.set("Content-Type", "application/json");
  }

  const response = await fetch(path, {
    ...options,
    headers,
    credentials: "same-origin",
  });

  const contentType = response.headers.get("content-type") || "";
  let body: any = null;
  if (contentType.includes("application/json")) {
    body = await response.json();
  }

  if (!response.ok) {
    const code = body?.error?.code || "api_error";
    const message = body?.error?.message || response.statusText || "Request failed";
    throw new ApiError(code, message, response.status);
  }

  return (body?.data ?? body) as T;
}

export async function apiRegister(email: string, password: string): Promise<AuthUser> {
  return request<AuthUser>("/api/v1/auth/register", {
    method: "POST",
    body: JSON.stringify({ email, password }),
  });
}

export async function apiLogin(email: string, password: string): Promise<AuthUser> {
  return request<AuthUser>("/api/v1/auth/login", {
    method: "POST",
    body: JSON.stringify({ email, password }),
  });
}

export async function apiLogout(): Promise<void> {
  await request<{ message: string }>("/api/v1/auth/logout", {
    method: "POST",
  });
}

export async function apiGetMe(): Promise<AuthUser> {
  return request<AuthUser>("/api/v1/auth/me", {
    method: "GET",
  });
}

export async function apiListModels(): Promise<string[]> {
  return request<string[]>("/api/v1/models", {
    method: "GET",
  });
}

export async function apiListGuestModels(): Promise<string[]> {
  return request<string[]>("/api/v1/guest/models", { method: "GET" });
}

export async function apiUploadAttachment(file: File): Promise<ApiAttachment> {
  const formData = new FormData();
  formData.append("file", file);

  return request<ApiAttachment>("/api/v1/attachments", {
    method: "POST",
    body: formData,
  });
}

export function getAttachmentContentUrl(id: string): string {
  return `/api/v1/attachments/${id}/content`;
}

export async function apiListConversations(limit = 50, offset = 0): Promise<ApiConversation[]> {
  return request<ApiConversation[]>(`/api/v1/conversations?limit=${limit}&offset=${offset}`, {
    method: "GET",
  });
}

export async function apiCreateConversation(title?: string): Promise<ApiConversation> {
  return request<ApiConversation>("/api/v1/conversations", {
    method: "POST",
    body: JSON.stringify({ title: title || "" }),
  });
}

export async function apiGetConversation(id: string): Promise<ApiConversationDetail> {
  return request<ApiConversationDetail>(`/api/v1/conversations/${id}`, {
    method: "GET",
  });
}

export async function apiRenameConversation(id: string, title: string): Promise<void> {
  await request<{ message: string }>(`/api/v1/conversations/${id}`, {
    method: "PATCH",
    body: JSON.stringify({ title }),
  });
}

export async function apiDeleteConversation(id: string): Promise<void> {
  await request<{ message: string }>(`/api/v1/conversations/${id}`, {
    method: "DELETE",
  });
}

export type StreamHandlers = {
  onCreated?: (data: { userMessage?: ApiMessage; message: ApiMessage }) => void;
  onDelta?: (delta: string, messageId: string) => void;
  onCompleted?: (message: ApiMessage) => void;
  onError?: (err: Error) => void;
};

export async function apiStreamMessage(
  conversationId: string,
  content: string,
  model: string,
  attachmentIds: string[],
  signal: AbortSignal,
  handlers: StreamHandlers
): Promise<void> {
  const idempotencyKey = crypto.randomUUID();

  const response = await fetch(`/api/v1/conversations/${conversationId}/messages:stream`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "Idempotency-Key": idempotencyKey,
    },
    credentials: "same-origin",
    body: JSON.stringify({
      content,
      model,
      attachmentIds,
    }),
    signal,
  });

  if (!response.ok) {
    let msg = "Gagal mengirim pesan";
    try {
      const errJson = await response.json();
      if (errJson?.error?.message) msg = errJson.error.message;
    } catch {
      // ignore
    }
    const err = new Error(msg);
    handlers.onError?.(err);
    throw err;
  }

  const reader = response.body?.getReader();
  if (!reader) throw new Error("Body reader tidak tersedia");

  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split("\n\n");
    buffer = lines.pop() || "";

    for (const block of lines) {
      if (!block.trim()) continue;
      const eventMatch = block.match(/event:\s*([^\n]+)/);
      const dataMatch = block.match(/data:\s*([^\n]+)/);

      const event = eventMatch ? eventMatch[1].trim() : "message";
      const rawData = dataMatch ? dataMatch[1].trim() : "";

      if (!rawData) continue;

      try {
        const parsed = JSON.parse(rawData);
        if (event === "message.created") {
          handlers.onCreated?.(parsed);
        } else if (event === "message.delta") {
          handlers.onDelta?.(parsed.delta, parsed.messageId);
        } else if (event === "message.completed") {
          handlers.onCompleted?.(parsed.message);
        } else if (event === "message.failed") {
          const err = new Error(parsed?.error?.message || "Terjadi kesalahan stream");
          handlers.onError?.(err);
        }
      } catch {
        // ignore parse errors
      }
    }
  }
}

export async function apiStreamGuestMessage(
  conversationId: string,
  content: string,
  model: string,
  history: Array<{ role: Role; content: string }>,
  signal: AbortSignal,
  handlers: StreamHandlers
): Promise<void> {
  const response = await fetch(`/api/v1/guest/chat/${conversationId}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ content, model, history }),
    signal,
  });
  if (!response.ok) {
    const body = await response.json().catch(() => null);
    throw new Error(body?.error?.message || "Gagal mengirim pesan");
  }
  const reader = response.body?.getReader();
  if (!reader) throw new Error("Body reader tidak tersedia");
  const decoder = new TextDecoder();
  let buffer = "";
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const blocks = buffer.split("\n\n");
    buffer = blocks.pop() || "";
    for (const block of blocks) {
      const event = block.match(/event:\s*([^\n]+)/)?.[1].trim();
      const rawData = block.match(/data:\s*([^\n]+)/)?.[1].trim();
      if (!rawData) continue;
      const parsed = JSON.parse(rawData);
      if (event === "message.created") handlers.onCreated?.(parsed);
      else if (event === "message.delta") handlers.onDelta?.(parsed.delta, parsed.messageId);
      else if (event === "message.completed") handlers.onCompleted?.(parsed.message);
      else if (event === "message.failed") handlers.onError?.(new Error(parsed?.error?.message || "Terjadi kesalahan stream"));
    }
  }
}

export async function apiRegenerateMessage(
  conversationId: string,
  messageId: string,
  signal: AbortSignal,
  handlers: StreamHandlers
): Promise<void> {
  const response = await fetch(`/api/v1/conversations/${conversationId}/messages/${messageId}:regenerate`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    signal,
  });

  if (!response.ok) {
    let msg = "Gagal membuat ulang jawaban";
    try {
      const errJson = await response.json();
      if (errJson?.error?.message) msg = errJson.error.message;
    } catch {
      // ignore
    }
    const err = new Error(msg);
    handlers.onError?.(err);
    throw err;
  }

  const reader = response.body?.getReader();
  if (!reader) throw new Error("Body reader tidak tersedia");

  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split("\n\n");
    buffer = lines.pop() || "";

    for (const block of lines) {
      if (!block.trim()) continue;
      const eventMatch = block.match(/event:\s*([^\n]+)/);
      const dataMatch = block.match(/data:\s*([^\n]+)/);

      const event = eventMatch ? eventMatch[1].trim() : "message";
      const rawData = dataMatch ? dataMatch[1].trim() : "";

      if (!rawData) continue;

      try {
        const parsed = JSON.parse(rawData);
        if (event === "message.created") {
          handlers.onCreated?.(parsed);
        } else if (event === "message.delta") {
          handlers.onDelta?.(parsed.delta, parsed.messageId);
        } else if (event === "message.completed") {
          handlers.onCompleted?.(parsed.message);
        } else if (event === "message.failed") {
          const err = new Error(parsed?.error?.message || "Terjadi kesalahan stream");
          handlers.onError?.(err);
        }
      } catch {
        // ignore parse errors
      }
    }
  }
}

export async function apiCancelMessage(conversationId: string, messageId: string): Promise<void> {
  await fetch(`/api/v1/conversations/${conversationId}/messages/${messageId}:cancel`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
  });
}
