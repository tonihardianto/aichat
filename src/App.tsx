import { useEffect, useRef, useState } from "react";
import { LogIn, Menu, MessageSquare } from "lucide-react";
import { useChat } from "@/hooks/useChat";
import { useConversations } from "@/hooks/useConversations";
import { useAuth } from "@/hooks/useAuth";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
import { Sidebar } from "@/components/chat/Sidebar";
import { MessageList } from "@/components/chat/MessageList";
import { Composer } from "@/components/chat/Composer";
import { ModelSelect } from "@/components/chat/ModelSelect";
import { AuthModal } from "@/components/auth/AuthModal";
import { ExtensionsDialog } from "@/components/chat/ExtensionsDialog";
import { apiGenerateTitle } from "@/lib/api";
import { titleFromMessages } from "@/lib/title";
import { UNTITLED_CONVERSATION } from "@/types/chat";
import { resolveTheme, type ThemePreference } from "@/lib/theme";
import { useModels } from "@/hooks/useModels";

import { TooltipProvider } from "@/components/ui/tooltip";

const THEME_KEY = "ai-chat-theme";

function initialTheme(): ThemePreference {
  const stored = localStorage.getItem(THEME_KEY);
  return stored === "light" || stored === "dark" || stored === "system" ? stored : "system";
}

export default function App() {
  const auth = useAuth();
  const conversations = useConversations(Boolean(auth.user));
  const models = useModels(Boolean(auth.user));
  const chat = useChat(conversations.activeConversation, conversations.updateConversation, models.selectedModel, !auth.user);
  const [mobileOpen, setMobileOpen] = useState(false);
  const [authOpen, setAuthOpen] = useState(false);
  const [extensionsOpen, setExtensionsOpen] = useState(false);
  const [theme, setTheme] = useState<ThemePreference>(initialTheme);

  useEffect(() => {
    if (auth.isLoading) return;
    void models.reload();
  }, [auth.user, auth.isLoading, models.reload]);

  // Auto-title: name an untitled conversation after its first user message.
  const { id: activeId, title: activeTitle, messages: activeMessages } = conversations.activeConversation;
  const renameConversation = conversations.rename;
  useEffect(() => {
    if (activeTitle !== UNTITLED_CONVERSATION) return;
    const title = titleFromMessages(activeMessages);
    if (title) void renameConversation(activeId, title);
  }, [activeId, activeTitle, activeMessages, renameConversation]);

  // Upgrade the quick title to an AI-written one after the first exchange (logged-in users).
  const titleTried = useRef(new Set<string>());
  const { isStreaming } = chat;
  const loggedIn = Boolean(auth.user);
  const selectedModel = models.selectedModel;
  useEffect(() => {
    if (!loggedIn || isStreaming || !selectedModel) return;
    if (activeMessages.length !== 2 || titleTried.current.has(activeId)) return;
    const [first, reply] = activeMessages;
    if (first.role !== "user" || reply.role !== "assistant" || !reply.content.trim()) return;
    if (activeTitle !== titleFromMessages(activeMessages)) return; // user renamed it manually
    titleTried.current.add(activeId);
    apiGenerateTitle(activeId, selectedModel)
      .then((title) => renameConversation(activeId, title))
      .catch(() => {
        /* keep the quick title */
      });
  }, [loggedIn, isStreaming, selectedModel, activeId, activeTitle, activeMessages, renameConversation]);

  useEffect(() => {
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const apply = () => (document.documentElement.className = resolveTheme(theme, media.matches));
    apply();
    media.addEventListener("change", apply);
    localStorage.setItem(THEME_KEY, theme);
    return () => media.removeEventListener("change", apply);
  }, [theme]);

  const sidebarProps = {
    conversations: conversations.conversations,
    activeId: conversations.activeId,
    onSelect: (id: string) => {
      conversations.select(id);
      setMobileOpen(false);
      chat.clearError();
    },
    onNew: () => {
      conversations.createConversation();
      chat.clearError();
      setMobileOpen(false);
    },
    onRename: conversations.rename,
    onDelete: (id: string) => {
      conversations.remove(id);
      chat.clearError();
    },
    user: auth.user,
    onLogout: () => {
      void auth.logout();
    },
    onOpenAuth: () => setAuthOpen(true),
    onOpenExtensions: () => setExtensionsOpen(true),
  };

  const empty = conversations.activeConversation.messages.length === 0;

  return (
    <TooltipProvider delayDuration={200}>
      <main className="flex h-dvh min-h-[360px] min-w-0 overflow-hidden bg-background text-foreground">
        <div className="hidden w-64 shrink-0 border-r md:block">
          <Sidebar {...sidebarProps} />
        </div>
        <section aria-label="Chat" className="flex min-w-0 flex-1 flex-col">
          <header className="flex min-h-14 shrink-0 flex-wrap items-center gap-2 border-b bg-background/80 px-3 py-2 backdrop-blur md:px-5">
            <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
              <SheetTrigger asChild>
                <Button className="md:hidden" variant="ghost" size="icon" aria-label="Buka riwayat">
                  <Menu />
                </Button>
              </SheetTrigger>
              <SheetContent side="left" className="w-[min(85vw,320px)] p-0">
                <SheetTitle className="sr-only">Riwayat percakapan</SheetTitle>
                <Sidebar {...sidebarProps} />
              </SheetContent>
            </Sheet>
            <div className="flex size-8 items-center justify-center rounded-lg bg-primary text-primary-foreground md:hidden">
              <MessageSquare className="size-4" />
            </div>
            <h1 className="truncate text-sm font-semibold md:hidden">AI Chat</h1>
            <div className="ml-auto flex max-w-full flex-wrap items-center justify-end gap-2">
              {!auth.user && (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => setAuthOpen(true)}
                  className="h-9 gap-1.5 text-xs"
                >
                  <LogIn className="size-3.5" />
                  <span>Masuk</span>
                </Button>
              )}
            <ModelSelect
                models={models.models}
                value={models.selectedModel}
                loading={models.isLoading}
                error={models.error}
                isDemo={models.isDemo}
                onChange={models.setSelectedModel}
                onReload={models.reload}
                guestMode={!auth.user}
              />
              <label className="sr-only" htmlFor="tema">
                Tema
              </label>
              <select
                id="tema"
                value={theme}
                onChange={(event) => setTheme(event.target.value as ThemePreference)}
                className="h-9 rounded-lg border bg-background px-3 text-xs outline-none transition-colors hover:bg-accent/50"
              >
                <option value="system">Sistem</option>
                <option value="light">Terang</option>
                <option value="dark">Gelap</option>
              </select>
            </div>
          </header>
          <MessageList
            messages={conversations.activeConversation.messages}
            isStreaming={chat.isStreaming}
            onRegenerate={chat.regenerate}
            onSuggestion={(value) => {
              void chat.send(value);
            }}
          />
          <div className={`shrink-0 ${empty ? "mt-auto" : ""}`}>
            {chat.error && (
              <div
                role="alert"
                className="mx-auto mb-3 flex max-w-[760px] items-center justify-between gap-3 rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive"
              >
                <span>{chat.error}</span>
                <Button variant="outline" size="sm" onClick={chat.retry}>
                  Coba lagi
                </Button>
              </div>
            )}
            <Composer
              onSend={chat.send}
              onStop={chat.stop}
              isStreaming={chat.isStreaming}
              centered={empty}
              models={models.models}
              selectedModel={models.selectedModel}
              onModelChange={models.setSelectedModel}
              guestMode={!auth.user}
            />
          </div>
        </section>
      </main>

      <AuthModal
        open={authOpen}
        onClose={() => setAuthOpen(false)}
        onLogin={auth.login}
        onRegister={auth.register}
      />
      <ExtensionsDialog open={extensionsOpen} onClose={() => setExtensionsOpen(false)} />
    </TooltipProvider>
  );
}
