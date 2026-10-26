import { useState } from "react";
import { Menu, MessageSquare } from "lucide-react";
import { useChat } from "@/hooks/useChat";
import { useConversations } from "@/hooks/useConversations";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
import { Sidebar } from "@/components/chat/Sidebar";
import { MessageList } from "@/components/chat/MessageList";
import { Composer } from "@/components/chat/Composer";

export default function App() {
  const conversations = useConversations();
  const chat = useChat(conversations.activeConversation, conversations.updateConversation);
  const [mobileOpen, setMobileOpen] = useState(false);
  const sidebarProps = {
    conversations: conversations.conversations,
    activeId: conversations.activeId,
    onSelect: (id: string) => { conversations.select(id); setMobileOpen(false); chat.clearError(); },
    onNew: () => { conversations.createConversation(); chat.clearError(); setMobileOpen(false); },
    onRename: conversations.rename,
    onDelete: (id: string) => { conversations.remove(id); chat.clearError(); },
  };
  const empty = conversations.activeConversation.messages.length === 0;
  return <main className="flex h-dvh min-h-[360px] min-w-[360px] overflow-hidden bg-background text-foreground">
    <div className="hidden w-64 shrink-0 border-r md:block"><Sidebar {...sidebarProps} /></div>
    <section aria-label="Chat" className="flex min-w-0 flex-1 flex-col">
      <header className="flex h-14 shrink-0 items-center gap-3 border-b px-3 md:px-5">
        <Sheet open={mobileOpen} onOpenChange={setMobileOpen}><SheetTrigger asChild><Button className="md:hidden" variant="ghost" size="icon" aria-label="Buka riwayat"><Menu /></Button></SheetTrigger><SheetContent side="left" className="w-[min(85vw,320px)] p-0"><SheetTitle className="sr-only">Riwayat percakapan</SheetTitle><Sidebar {...sidebarProps} /></SheetContent></Sheet>
        <div className="flex size-8 items-center justify-center rounded-lg bg-blue-600 text-white"><MessageSquare className="size-4" /></div><div className="min-w-0"><h1 className="truncate text-sm font-semibold">AI Chat</h1><p className="text-xs text-muted-foreground">Asisten percakapan</p></div>
      </header>
      <MessageList messages={conversations.activeConversation.messages} isStreaming={chat.isStreaming} onRegenerate={chat.regenerate} />
      <div className={`shrink-0 ${empty ? "mt-auto" : ""}`}>
        {chat.error && <div role="alert" className="mx-auto mb-3 flex max-w-[760px] items-center justify-between gap-3 rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive"><span>{chat.error}</span><Button variant="outline" size="sm" onClick={chat.retry}>Coba lagi</Button></div>}
        <Composer onSend={chat.send} onStop={chat.stop} isStreaming={chat.isStreaming} centered={empty} />
      </div>
    </section>
  </main>;
}
