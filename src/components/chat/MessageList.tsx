import { useEffect, useRef } from "react";
import type { Message } from "@/types/chat";
import { MessageBubble } from "@/components/chat/MessageBubble";

type Props = { messages: Message[]; isStreaming: boolean; onRegenerate: () => void };

export function MessageList({ messages, isStreaming, onRegenerate }: Props) {
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
    {messages.length === 0 ? <div className="flex h-full min-h-48 items-center justify-center text-center text-2xl font-medium tracking-tight text-slate-500">Apa yang ingin Anda kerjakan?</div> : <div className="mx-auto flex w-full max-w-[760px] flex-col gap-5 py-8">{messages.map((message, index) => <MessageBubble key={message.id} message={message} streaming={isStreaming && index === messages.length - 1 && message.role === "assistant"} onRegenerate={!isStreaming && index === messages.length - 1 && message.role === "assistant" ? onRegenerate : undefined} />)}</div>}
  </div>;
}
