import { useEffect, useRef, useState } from "react";
import { ArrowUp, ImagePlus, Square, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import type { ImageAttachment } from "@/types/chat";

type Props = { onSend: (value: string, image?: ImageAttachment) => Promise<void>; onStop: () => void; isStreaming: boolean; centered?: boolean };

export function Composer({ onSend, onStop, isStreaming, centered }: Props) {
  const [value, setValue] = useState("");
  const [image, setImage] = useState<ImageAttachment>();
  const [imageError, setImageError] = useState("");
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const imageReadId = useRef(0);
  const send = async () => {
    if (isStreaming || (!value.trim() && !image)) return;
    try {
      await onSend(value.trim(), image);
      imageReadId.current++;
      setValue(""); setImage(undefined); setImageError("");
      if (inputRef.current) inputRef.current.value = "";
    } catch { /* Keep draft if send fails. */ }
  };
  const readImage = (event: React.ChangeEvent<HTMLInputElement>) => {
    const readId = ++imageReadId.current;
    const file = event.target.files?.[0];
    if (!file) return;
    if (!["image/jpeg", "image/png", "image/webp"].includes(file.type)) {
      setImageError("Format gambar harus JPEG, PNG, atau WebP."); event.target.value = ""; return;
    }
    if (file.size > 5 * 1024 * 1024) {
      setImageError("Ukuran gambar maksimal 5 MB."); event.target.value = ""; return;
    }
    const reader = new FileReader();
    reader.onload = () => {
      if (readId !== imageReadId.current) return;
      if (typeof reader.result === "string") {
        setImage({ name: file.name, type: file.type as ImageAttachment["type"], dataUrl: reader.result });
        setImageError("");
      } else setImageError("Gambar gagal dibaca. Coba lagi.");
    };
    reader.onerror = () => { if (readId === imageReadId.current) setImageError("Gambar gagal dibaca. Coba lagi."); };
    reader.readAsDataURL(file);
  };
  useEffect(() => { const input = textareaRef.current; if (input) { input.style.height = "auto"; input.style.height = `${Math.min(input.scrollHeight, 200)}px`; } }, [value]);
  return <form onSubmit={(event) => { event.preventDefault(); send(); }} className={`w-full px-4 pb-3 ${centered ? "" : ""}`}>
      <div className="mx-auto max-w-[760px] rounded-2xl border bg-background shadow-sm focus-within:ring-2 focus-within:ring-ring/40">
        {image && <div className="relative m-3 w-fit"><img src={image.dataUrl} alt={`Lampiran ${image.name}`} className="max-h-40 max-w-48 rounded-md object-contain" /><span className="mt-1 block max-w-48 truncate text-xs text-muted-foreground">{image.name}</span><Button type="button" size="icon" variant="secondary" aria-label="Hapus gambar" onClick={() => { imageReadId.current++; setImage(undefined); if (inputRef.current) inputRef.current.value = ""; }} className="absolute -right-2 -top-2 size-7 rounded-full"><X className="size-4" /></Button></div>}
        <input ref={inputRef} className="sr-only" type="file" accept="image/jpeg,image/png,image/webp" onChange={readImage} />
        <Textarea ref={textareaRef} value={value} maxLength={8000} rows={1} aria-label="Tulis pesan" placeholder="Ketik pesan..." onChange={(event) => setValue(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter" && !event.shiftKey) { event.preventDefault(); send(); } }} className="max-h-[200px] min-h-12 resize-none border-0 px-4 py-3 shadow-none focus-visible:ring-0" />
        <div className="flex min-h-12 items-center justify-between px-3 pb-2">
          <span aria-live="polite" className={`text-xs text-muted-foreground ${value.length >= 7500 ? "visible" : "invisible"}`}>{value.length}/8000</span>
          <div className="flex items-center gap-2"><Button type="button" size="icon" variant="ghost" aria-label="Lampirkan gambar" onClick={() => inputRef.current?.click()} disabled={isStreaming}><ImagePlus className="size-4" /></Button>{isStreaming ? <Button type="button" size="icon" variant="secondary" aria-label="Hentikan" onClick={onStop}><Square className="size-4 fill-current" /></Button> : <Button type="submit" size="icon" aria-label="Kirim" disabled={!value.trim() && !image}><ArrowUp /></Button>}</div>
        </div>
        {imageError && <p role="alert" className="px-4 pb-2 text-sm text-destructive">{imageError}</p>}
    </div>
    <p className="mt-2 text-center text-[11px] text-muted-foreground">AI dapat membuat kesalahan. Periksa informasi penting.</p>
  </form>;
}
