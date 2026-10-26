import { useState } from "react";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import { Check, Copy, RefreshCw } from "lucide-react";
import type { Message } from "@/types/chat";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";

type Props = { message: Message; streaming?: boolean; onRegenerate?: () => void };

function CopyButton({ value, label = "Salin" }: { value: string; label?: string }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try { await navigator.clipboard.writeText(value); setCopied(true); window.setTimeout(() => setCopied(false), 1500); } catch { setCopied(false); }
  };
  return <Tooltip><TooltipTrigger asChild><Button type="button" size="icon" variant="ghost" aria-label={label} onClick={copy} className="size-8">{copied ? <Check /> : <Copy />}</Button></TooltipTrigger><TooltipContent>{copied ? "Tersalin" : label}</TooltipContent></Tooltip>;
}

const markdownComponents: Components = {
  code({ className, children, ...props }) {
    const block = Boolean(className);
    const value = String(children).replace(/\n$/, "");
    return block ? <div className="my-3 overflow-hidden rounded-lg border bg-slate-950 text-slate-100"><div className="flex items-center justify-between border-b border-white/10 px-3 py-1 text-xs text-slate-300"><span>{className?.replace("language-", "") || "Kode"}</span><CopyButton value={value} /></div><pre className="overflow-x-auto p-4 text-sm"><code className={className} {...props}>{children}</code></pre></div> : <code className="rounded bg-muted px-1.5 py-0.5 text-[0.9em]" {...props}>{children}</code>;
  },
  a: ({ ...props }) => <a {...props} className="text-primary underline underline-offset-4" />,
  table: ({ ...props }) => <div className="overflow-x-auto"><table {...props} /></div>,
};

export function MessageBubble({ message, streaming, onRegenerate }: Props) {
  const user = message.role === "user";
  return <article aria-label={user ? "Pesan Anda" : "Jawaban asisten"} className={`group flex w-full ${user ? "justify-end" : "justify-start"}`}>
    <div className={`min-w-0 max-w-[min(88%,760px)] ${user ? "rounded-2xl bg-blue-600 px-4 py-3 text-white" : "w-full py-2"}`}>
      {message.content ? <div className={`markdown-content break-words ${user ? "[&_a]:text-white [&_code]:text-foreground" : ""}`}><ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>{message.content}</ReactMarkdown></div> : streaming ? <div className="flex h-8 items-center gap-1" role="status" aria-label="Menulis"><span className="typing-dot"/><span className="typing-dot [animation-delay:150ms]"/><span className="typing-dot [animation-delay:300ms]"/></div> : null}
      {!user && message.content && <TooltipProvider delayDuration={300}><div className="mt-1 flex items-center opacity-100 sm:opacity-0 sm:group-hover:opacity-100 sm:group-focus-within:opacity-100"><CopyButton value={message.content} />{onRegenerate && <Tooltip><TooltipTrigger asChild><Button type="button" size="icon" variant="ghost" aria-label="Buat ulang jawaban" onClick={onRegenerate} className="size-8"><RefreshCw /></Button></TooltipTrigger><TooltipContent>Buat ulang jawaban</TooltipContent></Tooltip>}</div></TooltipProvider>}
    </div>
  </article>;
}
