import { useCallback, useEffect, useRef, useState } from "react";
import {
  UNTITLED_CONVERSATION,
  type Conversation,
  type Message,
} from "@/types/chat";
import {
  apiCreateConversation,
  apiDeleteConversation,
  apiGetConversation,
  apiListConversations,
  apiRenameConversation,
  getAttachmentContentUrl,
  type ApiMessage,
} from "@/lib/api";

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

function mapApiMessage(m: ApiMessage): Message {
  let image = undefined;
  if (m.attachments && m.attachments.length > 0) {
    const first = m.attachments[0];
    image = {
      id: first.id,
      name: first.name,
      type: first.contentType as any,
      dataUrl: getAttachmentContentUrl(first.id),
    };
  }
  return {
    id: m.id,
    role: m.role,
    content: m.content,
    status: m.status,
    image,
    outputTokens: m.outputTokens,
    tokensPerSecond: m.tokensPerSecond,
    createdAt: m.createdAt,
  };
}

export function useConversations(isAuthenticated = true) {
  const [conversations, setConversations] = useState<Conversation[]>([createConversation()]);
  const [activeId, setActiveId] = useState<string>(() => conversations[0]?.id || "");
  const [activeConversation, setActiveConversation] = useState<Conversation>(() => conversations[0] || createConversation());
  const [isLoading, setIsLoading] = useState(false);
  const isMounted = useRef(true);
  const wasAuthenticated = useRef(isAuthenticated);

  useEffect(() => {
    if (wasAuthenticated.current === isAuthenticated) return;
    wasAuthenticated.current = isAuthenticated;
    const guest = createConversation();
    setConversations([guest]);
    setActiveId(guest.id);
    setActiveConversation(guest);
  }, [isAuthenticated]);

  // Load conversation list from server
  const loadConversations = useCallback(async () => {
    if (!isAuthenticated) return;
    setIsLoading(true);
    try {
      const list = await apiListConversations();
      if (!isMounted.current) return;
      if (list && list.length > 0) {
        const mapped: Conversation[] = list.map((item) => ({
          id: item.id,
          title: item.title,
          messages: [],
          createdAt: item.createdAt,
          updatedAt: item.updatedAt,
        }));
        setConversations(mapped);
        setActiveId((prev) => {
          if (mapped.some((c) => c.id === prev)) return prev;
          return mapped[0].id;
        });
      } else {
        // No conversations on server, create one
        try {
          const newConv = await apiCreateConversation(UNTITLED_CONVERSATION);
          const mapped: Conversation = {
            id: newConv.id,
            title: newConv.title,
            messages: [],
            createdAt: newConv.createdAt,
            updatedAt: newConv.updatedAt,
          };
          setConversations([mapped]);
          setActiveId(mapped.id);
        } catch {
          // ignore
        }
      }
    } catch {
      // ignore server load failures
    } finally {
      if (isMounted.current) setIsLoading(false);
    }
  }, [isAuthenticated]);

  useEffect(() => {
    isMounted.current = true;
    void loadConversations();
    return () => {
      isMounted.current = false;
    };
  }, [loadConversations]);

  // Load active conversation details when activeId changes
  useEffect(() => {
    if (!activeId || !isAuthenticated) return;

    let cancelled = false;
    apiGetConversation(activeId)
      .then((detail) => {
        if (cancelled) return;
        const conv: Conversation = {
          id: detail.id,
          title: detail.title,
          messages: (detail.messages || []).map(mapApiMessage),
          createdAt: detail.createdAt,
          updatedAt: detail.updatedAt,
        };
        setActiveConversation(conv);
        setConversations((prev) =>
          prev.map((c) => (c.id === conv.id ? { ...c, title: conv.title, messages: conv.messages } : c))
        );
      })
      .catch(() => {
        // fallback to memory
      });

    return () => {
      cancelled = true;
    };
  }, [activeId, isAuthenticated]);

  const create = useCallback(async () => {
    if (isAuthenticated) {
      try {
        const res = await apiCreateConversation(UNTITLED_CONVERSATION);
        const conv: Conversation = {
          id: res.id,
          title: res.title,
          messages: [],
          createdAt: res.createdAt,
          updatedAt: res.updatedAt,
        };
        setConversations((prev) => [conv, ...prev]);
        setActiveId(conv.id);
        setActiveConversation(conv);
        return conv;
      } catch {
        // fallback
      }
    }
    const conv = createConversation();
    setConversations((prev) => [conv, ...prev]);
    setActiveId(conv.id);
    setActiveConversation(conv);
    return conv;
  }, [isAuthenticated]);

  const select = useCallback((id: string) => {
    setActiveId(id);
    const found = conversations.find((c) => c.id === id);
    if (found) {
      setActiveConversation(found);
    }
  }, [conversations]);

  const updateConversation = useCallback(
    (id: string, updater: (conversation: Conversation) => Conversation) => {
      setConversations((previous) =>
        previous.map((conv) => {
          if (conv.id !== id) return conv;
          const updated = updater(conv);
          const next = {
            ...updated,
            id: conv.id,
            updatedAt: new Date().toISOString(),
          };
          return next;
        })
      );
      setActiveConversation((prev) => {
        if (prev.id !== id) return prev;
        const updated = updater(prev);
        return { ...updated, id: prev.id, updatedAt: new Date().toISOString() };
      });
    },
    []
  );

  const rename = useCallback(
    async (id: string, title: string) => {
      const nextTitle = title.trim() || UNTITLED_CONVERSATION;
      setConversations((prev) =>
        prev.map((c) => (c.id === id ? { ...c, title: nextTitle, updatedAt: new Date().toISOString() } : c))
      );
      setActiveConversation((prev) => (prev.id === id ? { ...prev, title: nextTitle } : prev));

      if (isAuthenticated) {
        try {
          await apiRenameConversation(id, nextTitle);
        } catch {
          // ignore
        }
      }
    },
    [isAuthenticated]
  );

  const remove = useCallback(
    async (id: string) => {
      setConversations((prev) => {
        const remaining = prev.filter((c) => c.id !== id);
        const nextList = remaining.length > 0 ? remaining : [createConversation()];
        return nextList;
      });

      if (activeId === id) {
        const next = conversations.find((c) => c.id !== id) || createConversation();
        setActiveId(next.id);
        setActiveConversation(next);
      }

      if (isAuthenticated) {
        try {
          await apiDeleteConversation(id);
        } catch {
          // ignore
        }
      }
    },
    [activeId, conversations, isAuthenticated]
  );

  return {
    conversations,
    activeId,
    activeConversation,
    isLoading,
    createConversation: create,
    select,
    update: updateConversation,
    updateConversation,
    rename,
    remove,
    reload: loadConversations,
  };
}
