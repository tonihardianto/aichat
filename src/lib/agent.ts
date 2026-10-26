import type { Message } from "@/types/chat";

const DEMO_DELAY_MS = 35;

function abortError(): DOMException {
  return new DOMException("Dibatalkan", "AbortError");
}

function isAbortError(error: unknown): boolean {
  return error instanceof DOMException && error.name === "AbortError";
}

function throwIfAborted(signal: AbortSignal): void {
  if (signal.aborted) throw abortError();
}

function transportError(error: unknown, signal: AbortSignal): never {
  if (isAbortError(error)) throw error;
  if (signal.aborted) throw abortError();
  throw new Error("Gagal menghubungi agen.");
}

async function readStream(
  reader: ReadableStreamDefaultReader<Uint8Array>,
  signal: AbortSignal,
): Promise<ReadableStreamReadResult<Uint8Array>> {
  throwIfAborted(signal);

  try {
    return await reader.read();
  } catch (error) {
    return transportError(error, signal);
  }
}

async function cancelStream(
  reader: ReadableStreamDefaultReader<Uint8Array>,
  signal: AbortSignal,
): Promise<void> {
  try {
    await reader.cancel();
  } catch (error) {
    transportError(error, signal);
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
  let dataLines: string[] = [];
  let finished = false;

  const processEvent = (): boolean => {
    if (!dataLines.length) return false;

    const data = dataLines.join("\n");
    dataLines = [];

    if (!data.trim()) return false;
    if (data.trim() === "[DONE]") return true;

    throwIfAborted(signal);
    const token = tokenFromData(data);
    if (token) onToken(token);
    return false;
  };

  const processLine = (line: string): boolean => {
    const value = line.endsWith("\r") ? line.slice(0, -1) : line;
    if (!value) return processEvent();

    if (value === "data") {
      dataLines.push("");
    } else if (value.startsWith("data:")) {
      dataLines.push(value.slice(5).replace(/^ /, ""));
    }

    return false;
  };

  try {
    while (!finished) {
      const { done, value } = await readStream(reader, signal);
      if (done) {
        throwIfAborted(signal);
        break;
      }

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split("\n");
      buffer = lines.pop() ?? "";

      for (const line of lines) {
        if (processLine(line)) {
          finished = true;
          break;
        }
      }
    }

    if (finished) {
      await cancelStream(reader, signal);
      throwIfAborted(signal);
      return;
    }

    buffer += decoder.decode();
    if (buffer) {
      for (const line of buffer.split("\n")) {
        if (processLine(line)) {
          await cancelStream(reader, signal);
          throwIfAborted(signal);
          return;
        }
      }
    }

    if (processEvent()) {
      await cancelStream(reader, signal);
    }
    throwIfAborted(signal);
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
      const { done, value } = await readStream(reader, signal);
      if (done) {
        throwIfAborted(signal);
        break;
      }

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
    throwIfAborted(signal);
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
    transportError(error, signal);
  }

  if (!response.ok) {
    throw new Error(`Agen membalas dengan status ${response.status}.`);
  }

  if (!response.body) {
    throw new Error("Agen tidak mengirim isi respons.");
  }

  const contentType = (
    response.headers.get("content-type") ?? ""
  ).toLowerCase();

  if (contentType.includes("text/event-stream")) {
    await streamEventStream(response.body, signal, onToken);
    return;
  }

  await streamText(response.body, signal, onToken);
}
