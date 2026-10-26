import { useEffect, useRef, useState } from "react";
import { ArrowUp, Square } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";

type Props = { onSend: (value: string) => void; onStop: () => void; isStreaming: boolean; centered?: boolean };

export function Composer({ onSend, onStop, isStreaming, centered }: Props) {
  const [value, setValue] = useState("");
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const send = () => { if (!isStreaming && value.trim()) { onSend(value.trim()); setValue(""); } };
  useEffect(() => { const input = textareaRef.current; if (input) { input.style.height = "auto"; input.style.height = `${Math.min(input.scrollHeight, 200)}px`; } }, [value]);
  return <form onSubmit={(event) => { event.preventDefault(); send(); }} className={`w-full px-4 pb-3 ${centered ? "" : ""}`}>
    <div className="mx-auto max-w-[760px] rounded-2xl border bg-background shadow-sm focus-within:ring-2 focus-within:ring-ring/40">
      <Textarea ref={textareaRef} value={value} maxLength={8000} rows={1} aria-label="Tulis pesan" placeholder="Ketik pesan..." onChange={(event) => setValue(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter" && !event.shiftKey) { event.preventDefault(); send(); } }} className="max-h-[200px] min-h-12 resize-none border-0 px-4 py-3 shadow-none focus-visible:ring-0" />
      <div className="flex min-h-12 items-center justify-between px-3 pb-2">
        <span aria-live="polite" className={`text-xs text-muted-foreground ${value.length >= 7500 ? "visible" : "invisible"}`}>{value.length}/8000</span>
        {isStreaming ? <Button type="button" size="icon" variant="secondary" aria-label="Hentikan" onClick={onStop}><Square className="size-4 fill-current" /></Button> : <Button type="submit" size="icon" aria-label="Kirim" disabled={!value.trim()}><ArrowUp /></Button>}
      </div>
    </div>
    <p className="mt-2 text-center text-[11px] text-muted-foreground">AI dapat membuat kesalahan. Periksa informasi penting.</p>
  </form>;
}
