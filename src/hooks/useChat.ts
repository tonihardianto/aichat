import { useCallback, useEffect, useRef, useState } from "react";
import { streamReply } from "@/lib/agent";
import type { Conversation, Message } from "@/types/chat";

const MAX_MESSAGE_LENGTH = 8_000;
const CONNECTION_ERROR =
  "Gagal menghubungi agent. Periksa koneksi lalu coba lagi.";

export type UpdateConversation = (
  id: string,
  updater: (conversation: Conversation) => Conversation,
) => void;

export type UseChatResult = {
  send: (content: string) => void;
  stop: () => void;
  retry: () => void;
  regenerate: () => void;
  isStreaming: boolean;
  error: string | null;
  clearError: () => void;
};

type RetryState = {
  conversationId: string;
  messages: Message[];
};

function isAbortError(error: unknown): boolean {
  return (
    (error instanceof DOMException || error instanceof Error) &&
    error.name === "AbortError"
  );
}

export function useChat(
  conversation: Conversation,
  updateConversation: UpdateConversation,
): UseChatResult {
  const [isStreaming, setIsStreaming] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const conversationRef = useRef(conversation);
  const updateConversationRef = useRef(updateConversation);
  const controllerRef = useRef<AbortController | null>(null);
  const retryRef = useRef<RetryState | null>(null);

  conversationRef.current = conversation;
  updateConversationRef.current = updateConversation;

  useEffect(
    () => () => {
      controllerRef.current?.abort();
    },
    [],
  );

  const stream = useCallback(
    async (conversationId: string, messages: Message[]) => {
      const controller = new AbortController();
      const assistant: Message = {
        id: crypto.randomUUID(),
        role: "assistant",
        content: "",
        createdAt: new Date().toISOString(),
      };

      controllerRef.current = controller;
      updateConversationRef.current(conversationId, (current) => ({
        ...current,
        messages: [...current.messages, assistant],
      }));
      setIsStreaming(true);
      let completed = false;

      try {
        await streamReply(messages, controller.signal, (token) => {
          updateConversationRef.current(conversationId, (current) => ({
            ...current,
            messages: current.messages.map((message) =>
              message.id === assistant.id
                ? { ...message, content: message.content + token }
                : message,
            ),
          }));
        });
        completed = true;
      } catch (streamError) {
        if (isAbortError(streamError) || controller.signal.aborted) {
          updateConversationRef.current(conversationId, (current) => ({
            ...current,
            messages: current.messages.filter(
              (message) =>
                message.id !== assistant.id || message.content.length > 0,
            ),
          }));
        } else {
          retryRef.current = { conversationId, messages };
          setError(CONNECTION_ERROR);
        }
      } finally {
        if (completed) retryRef.current = null;
        if (controllerRef.current === controller) {
          controllerRef.current = null;
          setIsStreaming(false);
        }
      }
    },
    [],
  );

  const send = useCallback(
    (content: string) => {
      if (
        !content.trim() ||
        content.length > MAX_MESSAGE_LENGTH ||
        controllerRef.current
      ) {
        return;
      }

      const current = conversationRef.current;
      const timestamp = new Date().toISOString();
      const user: Message = {
        id: crypto.randomUUID(),
        role: "user",
        content,
        createdAt: timestamp,
      };

      retryRef.current = null;
      setError(null);
      updateConversationRef.current(current.id, (conversation) => ({
        ...conversation,
        messages: [...conversation.messages, user],
      }));
      void stream(current.id, [...current.messages, user]);
    },
    [stream],
  );

  const stop = useCallback(() => {
    controllerRef.current?.abort();
  }, []);

  const retry = useCallback(() => {
    const failed = retryRef.current;
    if (!failed || controllerRef.current) return;

    retryRef.current = null;
    setError(null);
    updateConversationRef.current(failed.conversationId, (current) => ({
      ...current,
      messages:
        current.messages.at(-1)?.role === "assistant"
          ? current.messages.slice(0, -1)
          : current.messages,
    }));
    void stream(failed.conversationId, failed.messages);
  }, [stream]);

  const regenerate = useCallback(() => {
    const current = conversationRef.current;
    const finalMessage = current.messages.at(-1);
    if (finalMessage?.role !== "assistant" || controllerRef.current) return;

    const messages = current.messages.slice(0, -1);
    if (messages.at(-1)?.role !== "user") return;

    retryRef.current = null;
    setError(null);
    updateConversationRef.current(current.id, (conversation) => ({
      ...conversation,
      messages: conversation.messages.slice(0, -1),
    }));
    void stream(current.id, messages);
  }, [stream]);

  const clearError = useCallback(() => {
    setError(null);
  }, []);

  return {
    send,
    stop,
    retry,
    regenerate,
    isStreaming,
    error,
    clearError,
  };
}
