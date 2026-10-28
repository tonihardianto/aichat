import { useEffect, useRef, useState } from "react";
import ReactMarkdown, { type Components } from "react-markdown";
import type { ElementContent } from "hast";
import remarkGfm from "remark-gfm";
import rehypeHighlight from "rehype-highlight";
import { Check, ChevronRight, Copy, RefreshCw, Sparkles } from "lucide-react";
import type { Message } from "@/types/chat";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { Bubble, BubbleContent } from "@/components/ui/bubble";
import { CodePreview } from "@/components/chat/CodePreview";
import { Message as MessageRoot, MessageContent, MessageFooter } from "@/components/ui/message";

type Props = { message: Message; streaming?: boolean; onRegenerate?: () => void };

function CopyButton({ value, label = "Salin" }: { value: string; label?: string }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try { await navigator.clipboard.writeText(value); setCopied(true); window.setTimeout(() => setCopied(false), 1500); } catch { setCopied(false); }
  };
  return <Tooltip><TooltipTrigger asChild><Button type="button" size="icon" variant="ghost" aria-label={label} onClick={copy} className="size-8">{copied ? <Check /> : <Copy />}</Button></TooltipTrigger><TooltipContent>{copied ? "Tersalin" : label}</TooltipContent></Tooltip>;
}

/** Live "thinking" indicator: no background, streams reasoning text, shows elapsed time. */
function Thinking({ reasoning, active }: { reasoning?: string; active: boolean }) {
  const [open, setOpen] = useState(false);
  const [seconds, setSeconds] = useState(0);
  const start = useRef<number | null>(null);
  const bodyRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!active) return;
    start.current ??= Date.now();
    const tick = () => setSeconds(Math.floor((Date.now() - (start.current ?? Date.now())) / 1000));
    tick();
    const id = window.setInterval(tick, 500);
    return () => window.clearInterval(id);
  }, [active]);

  useEffect(() => {
    if (active && bodyRef.current) bodyRef.current.scrollTop = bodyRef.current.scrollHeight;
  }, [reasoning, active]);

  const expanded = active || open;
  const hasText = Boolean(reasoning?.trim());
  if (!active && !hasText) return null;

  return (
    <div className="text-sm text-muted-foreground" role={active ? "status" : undefined} aria-label={active ? "Sedang berpikir" : undefined}>
      <button
        type="button"
        disabled={active || !hasText}
        onClick={() => setOpen((v) => !v)}
        aria-expanded={expanded}
        className="inline-flex items-center gap-1.5 rounded-md py-0.5 text-xs font-medium transition-colors enabled:hover:text-foreground"
      >
        {active ? <Sparkles className="size-3.5 animate-pulse text-primary" /> : <ChevronRight className={`size-3.5 transition-transform ${open ? "rotate-90" : ""}`} />}
        <span className={active ? "animate-pulse" : ""}>
          {active ? `Sedang berpikir… ${seconds}d` : `Proses berpikir${seconds ? ` · ${seconds}d` : ""}`}
        </span>
      </button>
      {expanded && hasText && (
        <div ref={bodyRef} className="mt-1.5 max-h-48 overflow-y-auto whitespace-pre-wrap break-words border-l-2 border-border pl-3 text-xs leading-relaxed">
          {reasoning}
        </div>
      )}
    </div>
  );
}

function nodeText(node: ElementContent): string {
  if (node.type === "text") return node.value;
  return "children" in node ? node.children.map(nodeText).join("") : "";
}

const markdownComponents: Components = {
  code({ className, children, node, ...props }) {
    const raw = nodeText(node as ElementContent);
    const value = raw.replace(/\n$/, "");
    const isMultiLine = raw.includes("\n");
    const hasLanguage = Boolean(className && className.startsWith("language-"));
    const block = hasLanguage || isMultiLine;
    const language = className?.replace("language-", "") || "Kode";

    return block ? (
      <div className="my-3 overflow-hidden rounded-xl border bg-[hsl(var(--code-bg))] text-foreground shadow-sm">
        <div className="flex items-center justify-between border-b bg-muted/60 px-3 py-1 text-xs text-muted-foreground">
          <span>{language}</span>
          <CopyButton value={value} />
        </div>
        <pre className="overflow-x-auto p-4 text-sm font-mono leading-relaxed">
          <code className={className} {...props}>
            {children}
          </code>
        </pre>
        {(language === "html" || language === "jsx") && <CodePreview language={language} code={value} />}
      </div>
    ) : (
      <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-[0.9em]" {...props}>
        {children}
      </code>
    );
  },
  a: ({ ...props }) => <a {...props} className="text-primary underline underline-offset-4" />,
  table: ({ ...props }) => <div className="overflow-x-auto"><table {...props} /></div>,
};

export function MessageBubble({ message, streaming, onRegenerate }: Props) {
  const user = message.role === "user";
  return (
    <TooltipProvider delayDuration={300}>
      <MessageRoot align={user ? "end" : "start"} aria-label={user ? "Pesan Anda" : "Jawaban asisten"} className="group">
    {!user && <div aria-hidden="true" className="mt-1 flex size-7 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-primary to-violet-500 text-primary-foreground"><Sparkles className="size-3.5" /></div>}
    <MessageContent className={user ? "items-end" : "py-1"}>
      {user ? <Bubble align="end" variant="tinted"><BubbleContent className="markdown-content break-words">
        {message.image && <img src={message.image.dataUrl} alt={`Lampiran ${message.image.name}`} className="mb-2 max-h-72 rounded-md object-contain" />}
        {message.content && <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[[rehypeHighlight, { detect: true, ignoreMissing: true }]]} components={markdownComponents}>{message.content}</ReactMarkdown>}
      </BubbleContent></Bubble> : <>
        <Thinking reasoning={message.reasoning} active={Boolean(streaming) && !message.content} />
        {message.content ? (
          <Bubble align="start" variant="ghost">
            <BubbleContent className="markdown-content break-words">
              <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[[rehypeHighlight, { detect: true, ignoreMissing: true }]]} components={markdownComponents}>
                {message.content}
              </ReactMarkdown>
              {streaming && (
                <span className="ml-1 inline-block h-4 w-1.5 translate-y-0.5 animate-pulse rounded-xs bg-primary" />
              )}
            </BubbleContent>
          </Bubble>
        ) : null}
        {message.outputTokens !== undefined && <p className="mt-2 text-xs text-muted-foreground">{message.outputTokens} token{message.tokensPerSecond ? ` · ${message.tokensPerSecond.toFixed(1)} token/detik` : ""}</p>}
        {message.content && <MessageFooter className="px-0 opacity-100 sm:opacity-0 sm:group-hover:opacity-100 sm:group-focus-within:opacity-100"><CopyButton value={message.content} />{onRegenerate && <Tooltip><TooltipTrigger asChild><Button type="button" size="icon" variant="ghost" aria-label="Buat ulang jawaban" onClick={onRegenerate} className="size-8"><RefreshCw /></Button></TooltipTrigger><TooltipContent>Buat ulang jawaban</TooltipContent></Tooltip>}</MessageFooter>}
      </>}
    </MessageContent>
  </MessageRoot>
</TooltipProvider>
  );
}
