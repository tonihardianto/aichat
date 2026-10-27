import { useCallback, useEffect, useRef, useState } from "react";
import { streamReply } from "@/lib/agent";
import {
  apiCancelMessage,
  apiRegenerateMessage,
  apiStreamMessage,
  apiStreamGuestMessage,
  apiUploadAttachment,
  type ApiMessage,
  type StreamHandlers,
} from "@/lib/api";
import type { Conversation, ImageAttachment, Message } from "@/types/chat";

const MAX_MESSAGE_LENGTH = 8_000;
const CONNECTION_ERROR =
  "Gagal menghubungi server. Periksa koneksi lalu coba lagi.";

export type UpdateConversation = (
  id: string,
  updater: (conversation: Conversation) => Conversation,
) => void;

export type UseChatResult = {
  send: (content: string, image?: ImageAttachment, think?: boolean) => Promise<void>;
  stop: () => void;
  retry: () => void;
  regenerate: () => void;
  isStreaming: boolean;
  error: string | null;
  clearError: () => void;
};

type RetryState = {
  conversationId: string;
  content: string;
  image?: ImageAttachment;
  messages: Message[];
};

function dataUrlToFile(dataUrl: string, filename: string): File {
  const arr = dataUrl.split(",");
  const mime = arr[0].match(/:(.*?);/)?.[1] || "image/png";
  const bstr = atob(arr[1]);
  let n = bstr.length;
  const u8arr = new Uint8Array(n);
  while (n--) {
    u8arr[n] = bstr.charCodeAt(n);
  }
  return new File([u8arr], filename, { type: mime });
}

export function useChat(
  conversation: Conversation,
  updateConversation: UpdateConversation,
  model: string | null,
  isGuest = false,
): UseChatResult {
  const [isStreaming, setIsStreaming] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const conversationRef = useRef(conversation);
  const updateConversationRef = useRef(updateConversation);
  const controllerRef = useRef<AbortController | null>(null);
  const currentAssistantIdRef = useRef<string | null>(null);
  const retryRef = useRef<RetryState | null>(null);

  conversationRef.current = conversation;
  updateConversationRef.current = updateConversation;

  useEffect(
    () => () => {
      controllerRef.current?.abort();
    },
    [],
  );

  const send = useCallback(
    async (content: string, image?: ImageAttachment, think = false) => {
      if (
        (!content.trim() && !image) ||
        (isGuest && !!image) ||
        (isGuest && !model) ||
        content.length > MAX_MESSAGE_LENGTH ||
        controllerRef.current
      ) {
        return;
      }

      const current = conversationRef.current;
      const timestamp = new Date().toISOString();
      const localUserMsgId = crypto.randomUUID();
      const user: Message = {
        id: localUserMsgId,
        role: "user",
        content,
        ...(image ? { image } : {}),
        createdAt: timestamp,
      };

      const localAssistantId = crypto.randomUUID();
      const assistant: Message = {
        id: localAssistantId,
        role: "assistant",
        content: "",
        status: "streaming",
        createdAt: new Date().toISOString(),
      };

      currentAssistantIdRef.current = localAssistantId;
      retryRef.current = null;
      setError(null);

      // Optimistically add user and placeholder
      updateConversationRef.current(current.id, (conv) => ({
        ...conv,
        messages: [...conv.messages, user, assistant],
      }));

      const controller = new AbortController();
      controllerRef.current = controller;
      setIsStreaming(true);

      try {
        let attachmentIds: string[] = [];
        if (image && !isGuest) {
          if (image.id) {
            attachmentIds.push(image.id);
          } else if (image.dataUrl) {
            try {
              const file = dataUrlToFile(image.dataUrl, image.name || "image.png");
              const uploaded = await apiUploadAttachment(file);
              image.id = uploaded.id;
              attachmentIds.push(uploaded.id);
            } catch {
              // ignore upload fail, try stream without attachment or fall back
            }
          }
        }

        const selectedModel = model || "llama3.2";
        const handlers: StreamHandlers = {
            onCreated: (data) => {
              if (data.message?.id) {
                currentAssistantIdRef.current = data.message.id;
              }
              updateConversationRef.current(current.id, (conv) => ({
                ...conv,
                messages: conv.messages.map((m) => {
                  if (m.id === localUserMsgId && data.userMessage?.id) {
                    return { ...m, id: data.userMessage.id };
                  }
                  if (m.id === localAssistantId && data.message?.id) {
                    return { ...m, id: data.message.id };
                  }
                  return m;
                }),
              }));
            },
            onDelta: (delta, msgId) => {
              updateConversationRef.current(current.id, (conv) => ({
                ...conv,
                messages: conv.messages.map((m) => {
                  const targetId = currentAssistantIdRef.current || localAssistantId;
                  if (m.id === targetId || m.id === msgId) {
                    return { ...m, content: m.content + delta };
                  }
                  return m;
                }),
              }));
            },
            onCompleted: (msg: ApiMessage) => {
              updateConversationRef.current(current.id, (conv) => ({
                ...conv,
                messages: conv.messages.map((m) => {
                  const targetId = currentAssistantIdRef.current || localAssistantId;
                  if (m.id === targetId || m.id === msg.id) {
                    return {
                      ...m,
                      id: msg.id,
                      content: msg.content,
                      status: "complete",
                      outputTokens: msg.outputTokens,
                      tokensPerSecond: msg.tokensPerSecond,
                    };
                  }
                  return m;
                }),
              }));
            },
            onError: (err) => {
              setError(err.message || CONNECTION_ERROR);
            },
          };
        if (isGuest) {
          await apiStreamGuestMessage(current.id, content, selectedModel, current.messages.map(({ role, content }) => ({ role, content })), think, controller.signal, handlers);
        } else {
          await apiStreamMessage(current.id, content, selectedModel, attachmentIds, think, controller.signal, handlers);
        }
      } catch (err: any) {
        if (controller.signal.aborted) {
          // Message cancelled
          updateConversationRef.current(current.id, (conv) => ({
            ...conv,
            messages: conv.messages.map((m) => {
              const targetId = currentAssistantIdRef.current || localAssistantId;
              if (m.id === targetId) {
                return { ...m, status: "cancelled" };
              }
              return m;
            }),
          }));
        } else {
          if (isGuest) {
            retryRef.current = { conversationId: current.id, content, messages: [...current.messages, user] };
            setError(err.message || CONNECTION_ERROR);
          } else try {
            await streamReply(
              [...current.messages, user],
              model,
              controller.signal,
              (token) => {
                updateConversationRef.current(current.id, (conv) => ({
                  ...conv,
                  messages: conv.messages.map((m) =>
                    m.id === localAssistantId
                      ? { ...m, content: m.content + token }
                      : m
                  ),
                }));
              }
            );
          } catch {
            retryRef.current = {
              conversationId: current.id,
              content,
              image,
              messages: [...current.messages, user],
            };
            setError(err.message || CONNECTION_ERROR);
          }
        }
      } finally {
        if (controllerRef.current === controller) {
          controllerRef.current = null;
          currentAssistantIdRef.current = null;
          setIsStreaming(false);
        }
      }
    },
    [model, isGuest]
  );

  const stop = useCallback(() => {
    if (controllerRef.current) {
      controllerRef.current.abort();
      const current = conversationRef.current;
      const asstId = currentAssistantIdRef.current;
      if (asstId && current?.id) {
        void apiCancelMessage(current.id, asstId);
      }
    }
  }, []);

  const retry = useCallback(() => {
    const failed = retryRef.current;
    if (!failed || controllerRef.current) return;

    retryRef.current = null;
    setError(null);
    void send(failed.content, failed.image);
  }, [send]);

  const regenerate = useCallback(() => {
    const current = conversationRef.current;
    const finalMessage = current.messages.at(-1);
    if (finalMessage?.role !== "assistant" || controllerRef.current) return;

    const controller = new AbortController();
    controllerRef.current = controller;
    setIsStreaming(true);
    setError(null);

    const localAssistantId = crypto.randomUUID();
    const assistant: Message = {
      id: localAssistantId,
      role: "assistant",
      content: "",
      status: "streaming",
      createdAt: new Date().toISOString(),
    };
    currentAssistantIdRef.current = localAssistantId;

    updateConversationRef.current(current.id, (conv) => ({
      ...conv,
      messages: [...conv.messages, assistant],
    }));

    void apiRegenerateMessage(current.id, finalMessage.id, controller.signal, {
      onCreated: (data) => {
        if (data.message?.id) {
          currentAssistantIdRef.current = data.message.id;
        }
      },
      onDelta: (delta, msgId) => {
        updateConversationRef.current(current.id, (conv) => ({
          ...conv,
          messages: conv.messages.map((m) => {
            const targetId = currentAssistantIdRef.current || localAssistantId;
            if (m.id === targetId || m.id === msgId) {
              return { ...m, content: m.content + delta };
            }
            return m;
          }),
        }));
      },
      onCompleted: (msg) => {
        updateConversationRef.current(current.id, (conv) => ({
          ...conv,
          messages: conv.messages.map((m) => {
            const targetId = currentAssistantIdRef.current || localAssistantId;
            if (m.id === targetId || m.id === msg.id) {
              return {
                ...m,
                id: msg.id,
                content: msg.content,
                status: "complete",
                outputTokens: msg.outputTokens,
                tokensPerSecond: msg.tokensPerSecond,
              };
            }
            return m;
          }),
        }));
      },
      onError: (err) => {
        setError(err.message || CONNECTION_ERROR);
      },
    }).finally(() => {
      if (controllerRef.current === controller) {
        controllerRef.current = null;
        currentAssistantIdRef.current = null;
        setIsStreaming(false);
      }
    });
  }, []);

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
