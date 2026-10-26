import type { Message } from "@/types/chat";

const DEMO_DELAY_MS = 35;

function throwIfAborted(signal: AbortSignal): void {
  if (signal.aborted) {
    throw new DOMException("Dibatalkan", "AbortError");
  }
}

function tokenFromValue(value: unknown): string {
  if (typeof value === "string") return value;
  if (!value || typeof value !== "object") return "";

  const record = value as Record<string, unknown>;
  for (const key of ["token", "delta", "content", "text"]) {
    const token = tokenFromValue(record[key]);
    if (token) return token;
  }

  return "";
}

function tokenFromData(data: string): string {
  try {
    return tokenFromValue(JSON.parse(data));
  } catch {
    return data;
  }
}

async function streamEventStream(
  body: ReadableStream<Uint8Array>,
  signal: AbortSignal,
  onToken: (token: string) => void,
): Promise<void> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  const handleLine = (line: string): boolean => {
    const value = line.endsWith("\r") ? line.slice(0, -1) : line;
    if (!value.startsWith("data:")) return false;

    const data = value.slice(5).replace(/^ /, "");
    if (data.trim() === "[DONE]") return true;
    if (!data.trim()) return false;

    throwIfAborted(signal);
    const token = tokenFromData(data);
    if (token) onToken(token);
    return false;
  };

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      // ponytail: one data line per event; join multi-line data if a provider splits tokens.
      const lines = buffer.split("\n");
      buffer = lines.pop() ?? "";

      for (const line of lines) {
        if (handleLine(line)) return;
      }
    }

    buffer += decoder.decode();
    for (const line of buffer.split("\n")) {
      handleLine(line);
    }
  } finally {
    reader.releaseLock();
  }
}

async function streamText(
  body: ReadableStream<Uint8Array>,
  signal: AbortSignal,
  onToken: (token: string) => void,
): Promise<void> {
  const reader = body.getReader();
  const decoder = new TextDecoder();

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      const chunk = decoder.decode(value, { stream: true });
      if (!chunk) continue;

      throwIfAborted(signal);
      onToken(chunk);
    }

    const tail = decoder.decode();
    if (tail) {
      throwIfAborted(signal);
      onToken(tail);
    }
  } finally {
    reader.releaseLock();
  }
}

export async function streamReply(
  messages: Message[],
  signal: AbortSignal,
  onToken: (token: string) => void,
): Promise<void> {
  const url = import.meta.env.VITE_AGENT_URL?.trim();

  if (!url) {
    const lastMessage = messages.at(-1)?.content ?? "";
    const reply = `Ini balasan demo untuk: "${lastMessage}".`;

    for (const word of reply.split(" ")) {
      throwIfAborted(signal);
      await new Promise((resolve) => setTimeout(resolve, DEMO_DELAY_MS));
      throwIfAborted(signal);
      onToken(`${word} `);
    }

    return;
  }

  let response: Response;
  try {
    response = await fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        messages: messages.map(({ role, content }) => ({ role, content })),
      }),
      signal,
    });
  } catch (error) {
    if (signal.aborted) throw error;
    throw new Error("Gagal menghubungi agen.");
  }

  if (!response.ok) {
    throw new Error(`Agen membalas dengan status ${response.status}.`);
  }

  if (!response.body) {
    throw new Error("Agen tidak mengirim isi respons.");
  }

  const contentType = response.headers.get("content-type") ?? "";

  if (contentType.includes("text/event-stream")) {
    await streamEventStream(response.body, signal, onToken);
    return;
  }

  await streamText(response.body, signal, onToken);
}
