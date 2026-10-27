import { useEffect, useRef } from "react";
import type { Message } from "@/types/chat";
import { MessageBubble } from "@/components/chat/MessageBubble";
import { Button } from "@/components/ui/button";
import { MessageGroup } from "@/components/ui/message";

const suggestions = ["Bantu saya menulis", "Jelaskan kode ini", "Beri ide baru"];

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
    {messages.length === 0 ? <div className="mx-auto flex h-full min-h-48 max-w-[920px] flex-col items-center justify-center gap-5 px-4 text-center">
      <div><h2 className="text-2xl font-semibold tracking-tight">Apa yang ingin kamu buat?</h2><p className="mt-2 text-sm text-muted-foreground">Tanyakan sesuatu atau pilih titik awal.</p></div>
      <div className="flex flex-wrap justify-center gap-2">{suggestions.map((suggestion) => <Button key={suggestion} type="button" variant="outline" size="sm" onClick={() => onSuggestion(suggestion)}>{suggestion}</Button>)}</div>
    </div> : <MessageGroup className="mx-auto w-full max-w-[920px] gap-4 py-5">{messages.map((message, index) => <MessageBubble key={message.id} message={message} streaming={isStreaming && index === messages.length - 1 && message.role === "assistant"} onRegenerate={!isStreaming && index === messages.length - 1 && message.role === "assistant" ? onRegenerate : undefined} />)}</MessageGroup>}
  </div>;
}
