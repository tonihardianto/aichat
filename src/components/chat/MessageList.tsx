import { Code2, Lightbulb, PenLine, Sparkles } from "lucide-react";
import { useEffect, useRef } from "react";
import type { Message } from "@/types/chat";
import { MessageBubble } from "@/components/chat/MessageBubble";
import { MessageGroup } from "@/components/ui/message";

const suggestions = [
  { text: "Bantu saya menulis", hint: "Email, artikel, atau draf", icon: PenLine },
  { text: "Jelaskan kode ini", hint: "Pahami dan perbaiki kode", icon: Code2 },
  { text: "Beri ide baru", hint: "Brainstorm tanpa batas", icon: Lightbulb },
];

type Props = { messages: Message[]; isStreaming: boolean; onRegenerate: () => void; onSuggestion: (value: string) => void };

export function MessageList({ messages, isStreaming, onRegenerate, onSuggestion }: Props) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const nearBottom = useRef(true);
  useEffect(() => {
    const element = scrollRef.current;
    if (element && nearBottom.current) element.scrollTop = element.scrollHeight;
  }, [messages]);
  return <div ref={scrollRef} role="log" aria-label="Pesan percakapan" aria-live="polite" onScroll={(event) => {
    const element = event.currentTarget;
    nearBottom.current = element.scrollHeight - element.scrollTop - element.clientHeight < 80;
  }} className="min-h-0 flex-1 overflow-y-auto px-4">
    {messages.length === 0 ? <div className="hero-glow mx-auto flex h-full min-h-48 max-w-[760px] flex-col items-center justify-center gap-8 px-4 text-center">
      <div className="flex flex-col items-center gap-4">
        <div className="flex size-12 items-center justify-center rounded-2xl bg-gradient-to-br from-primary to-violet-500 text-primary-foreground shadow-lg shadow-primary/25"><Sparkles className="size-6" /></div>
        <div><h2 className="text-3xl font-semibold tracking-tight">Apa yang ingin kamu buat?</h2><p className="mt-2 text-sm text-muted-foreground">Tanyakan sesuatu atau pilih titik awal.</p></div>
      </div>
      <div className="grid w-full gap-3 sm:grid-cols-3">{suggestions.map(({ text, hint, icon: Icon }) => <button key={text} type="button" onClick={() => onSuggestion(text)} className="group flex flex-col items-start gap-2 rounded-xl border bg-card p-4 text-left shadow-xs transition-all hover:-translate-y-0.5 hover:border-primary/40 hover:shadow-md"><Icon className="size-4 text-primary" /><span className="text-sm font-medium">{text}</span><span className="text-xs text-muted-foreground">{hint}</span></button>)}</div>
    </div> : <MessageGroup className="mx-auto w-full max-w-[920px] gap-4 py-5">{messages.map((message, index) => <MessageBubble key={message.id} message={message} streaming={isStreaming && index === messages.length - 1 && message.role === "assistant"} onRegenerate={!isStreaming && index === messages.length - 1 && message.role === "assistant" ? onRegenerate : undefined} />)}</MessageGroup>}
  </div>;
}
