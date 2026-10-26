import { useCallback, useEffect, useState } from "react";
import {
  UNTITLED_CONVERSATION,
  type Conversation,
  type Message,
} from "@/types/chat";

const STORAGE_KEY = "ai-chat-conversations-v1";
const TITLE_LENGTH = 40;

type Store = {
  conversations: Conversation[];
  activeId: string;
};

export function createConversation(): Conversation {
  const timestamp = new Date().toISOString();

  return {
    id: crypto.randomUUID(),
    title: UNTITLED_CONVERSATION,
    messages: [],
    createdAt: timestamp,
    updatedAt: timestamp,
  };
}

function isMessage(value: unknown): value is Message {
  if (!value || typeof value !== "object") return false;

  const message = value as Record<string, unknown>;

  return (
    typeof message.id === "string" &&
    (message.role === "user" || message.role === "assistant") &&
    typeof message.content === "string" &&
    typeof message.createdAt === "string"
  );
}

function isConversation(value: unknown): value is Conversation {
  if (!value || typeof value !== "object") return false;

  const conversation = value as Record<string, unknown>;

  return (
    typeof conversation.id === "string" &&
    typeof conversation.title === "string" &&
    Array.isArray(conversation.messages) &&
    conversation.messages.every(isMessage) &&
    typeof conversation.createdAt === "string" &&
    typeof conversation.updatedAt === "string"
  );
}

function readConversations(): Conversation[] {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (!stored) return [];

    const parsed: unknown = JSON.parse(stored);
    return Array.isArray(parsed) ? parsed.filter(isConversation) : [];
  } catch {
    return [];
  }
}

function titleFromMessages(conversation: Conversation): string {
  if (conversation.title !== UNTITLED_CONVERSATION) return conversation.title;

  const firstUserMessage = conversation.messages.find(
    (message) => message.role === "user",
  );
  const title = firstUserMessage?.content.trim().slice(0, TITLE_LENGTH);

  return title || UNTITLED_CONVERSATION;
}

export function useConversations() {
  const [store, setStore] = useState<Store>(() => {
    const stored = readConversations();
    const conversations = stored.length > 0 ? stored : [createConversation()];

    return { conversations, activeId: conversations[0].id };
  });

  useEffect(() => {
    try {
      localStorage.setItem(
        STORAGE_KEY,
        JSON.stringify(store.conversations),
      );
    } catch {
      // Storage can be unavailable or full; chat still works in memory.
    }
  }, [store.conversations]);

  const create = useCallback(() => {
    const conversation = createConversation();

    setStore((previous) => ({
      conversations: [conversation, ...previous.conversations],
      activeId: conversation.id,
    }));

    return conversation;
  }, []);

  const select = useCallback((id: string) => {
    setStore((previous) =>
      previous.conversations.some((conversation) => conversation.id === id)
        ? { ...previous, activeId: id }
        : previous,
    );
  }, []);

  const updateConversation = useCallback(
    (id: string, updater: (conversation: Conversation) => Conversation) => {
      setStore((previous) => ({
        ...previous,
        conversations: previous.conversations.map((conversation) => {
          if (conversation.id !== id) return conversation;

          const updated = updater(conversation);

          return {
            ...updated,
            id: conversation.id,
            title: titleFromMessages(updated),
            updatedAt: new Date().toISOString(),
          };
        }),
      }));
    },
    [],
  );

  const rename = useCallback((id: string, title: string) => {
    const nextTitle = title.trim() || UNTITLED_CONVERSATION;

    setStore((previous) => ({
      ...previous,
      conversations: previous.conversations.map((conversation) =>
        conversation.id === id
          ? {
              ...conversation,
              title: nextTitle,
              updatedAt: new Date().toISOString(),
            }
          : conversation,
      ),
    }));
  }, []);

  const remove = useCallback((id: string) => {
    setStore((previous) => {
      const remaining = previous.conversations.filter(
        (conversation) => conversation.id !== id,
      );
      const conversations =
        remaining.length > 0 ? remaining : [createConversation()];
      const activeId = conversations.some(
        (conversation) => conversation.id === previous.activeId,
      )
        ? previous.activeId
        : conversations[0].id;

      return { conversations, activeId };
    });
  }, []);

  const activeConversation =
    store.conversations.find(
      (conversation) => conversation.id === store.activeId,
    ) ?? store.conversations[0];

  return {
    conversations: store.conversations,
    activeId: store.activeId,
    activeConversation,
    createConversation: create,
    select,
    update: updateConversation,
    updateConversation,
    rename,
    remove,
  };
}
