import { useCallback, useEffect, useRef, useState } from "react";
import { ImagePlus, ImageIcon, Loader2, Sparkles, Trash2, Wand2, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ChatImage } from "@/components/chat/ChatImage";
import { ApiError, apiGenerateImages, apiImageGenConfig, type ImageGenConfig, type ImageGenParams } from "@/lib/api";

type HistoryItem = { url: string; prompt: string; model: string; createdAt: number };
type Settings = Omit<ImageGenParams, "prompt">;
type Reference = { id: string; file: File; preview: string };

const MAX_PROMPT = 4000;
const MAX_REF_BYTES = 10 * 1024 * 1024;
const REF_TYPES = ["image/png", "image/jpeg", "image/webp"];
const MAX_HISTORY = 24;
const DEFAULT_SETTINGS: Settings = {
  model: "",
  n: 1,
  size: "auto",
  quality: "auto",
  background: "auto",
  outputFormat: "png",
  watermark: true,
  promptExtend: true,
};

const SIZES = [
  ["auto", "Otomatis"],
  ["1024x1024", "Persegi (1024×1024)"],
  ["1536x1024", "Lanskap (1536×1024)"],
  ["1024x1536", "Potret (1024×1536)"],
];
const QUALITIES = [["auto", "Otomatis"], ["low", "Rendah"], ["medium", "Sedang"], ["high", "Tinggi"]];
const BACKGROUNDS = [["auto", "Otomatis"], ["transparent", "Transparan"], ["opaque", "Solid"]];
const FORMATS = [["png", "PNG"], ["jpeg", "JPEG"], ["webp", "WebP"]];

const fieldClass = "h-9 w-full rounded-lg border bg-background px-3 text-sm outline-none transition-colors hover:bg-accent/40 focus-visible:ring-1 focus-visible:ring-ring disabled:opacity-60";

function load<T>(key: string, fallback: T): T {
  try {
    const raw = localStorage.getItem(key);
    return raw ? (JSON.parse(raw) as T) : fallback;
  } catch {
    return fallback;
  }
}
function save(key: string, value: unknown) {
  try {
    localStorage.setItem(key, JSON.stringify(value));
  } catch {
    /* storage unavailable: keep in memory only */
  }
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block space-y-1.5">
      <span className="text-xs font-medium text-muted-foreground">{label}</span>
      {children}
    </label>
  );
}

function Select({ value, onChange, options, disabled }: { value: string; onChange: (v: string) => void; options: string[][]; disabled?: boolean }) {
  return (
    <select value={value} onChange={(e) => onChange(e.target.value)} disabled={disabled} className={fieldClass}>
      {options.map(([v, label]) => <option key={v} value={v}>{label}</option>)}
    </select>
  );
}

export function ImageGenerator({ userId }: { userId: string }) {
  const settingsKey = `ai-chat-image-settings:${userId}`;
  const historyKey = `ai-chat-image-history:${userId}`;

  const [config, setConfig] = useState<ImageGenConfig | null>(null);
  const [configError, setConfigError] = useState<string | null>(null);
  const [prompt, setPrompt] = useState("");
  const [settings, setSettings] = useState<Settings>(() => ({ ...DEFAULT_SETTINGS, ...load<Partial<Settings>>(settingsKey, {}) }));
  const [history, setHistory] = useState<HistoryItem[]>(() => load<HistoryItem[]>(historyKey, []));
  const [generating, setGenerating] = useState(false);
  const [seconds, setSeconds] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const [refs, setRefs] = useState<Reference[]>([]);
  const [dragging, setDragging] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const refsRef = useRef<Reference[]>([]);
  refsRef.current = refs;
  const maxRefs = config?.maxReferences ?? 4;

  useEffect(() => {
    apiImageGenConfig()
      .then((c) => {
        setConfig(c);
        setSettings((s) => (c.models.includes(s.model) ? s : { ...s, model: c.models[0] ?? "" }));
      })
      .catch(() => setConfigError("Gagal memuat pengaturan generator gambar."));
  }, []);

  useEffect(() => save(settingsKey, settings), [settings, settingsKey]);
  useEffect(() => save(historyKey, history), [history, historyKey]);
  useEffect(() => () => {
    abortRef.current?.abort();
    refsRef.current.forEach((r) => URL.revokeObjectURL(r.preview));
  }, []);

  const addFiles = useCallback((files: FileList | File[]) => {
    const list = Array.from(files);
    const problems: string[] = [];
    const accepted: Reference[] = [];
    for (const file of list) {
      if (!REF_TYPES.includes(file.type)) problems.push(`${file.name || "gambar"}: harus PNG, JPEG, atau WebP`);
      else if (file.size > MAX_REF_BYTES) problems.push(`${file.name || "gambar"}: lebih dari 10 MB`);
      else accepted.push({ id: crypto.randomUUID(), file, preview: URL.createObjectURL(file) });
    }
    setRefs((current) => {
      const room = Math.max(0, maxRefs - current.length);
      if (accepted.length > room) problems.push(`Maksimal ${maxRefs} gambar referensi`);
      accepted.slice(room).forEach((r) => URL.revokeObjectURL(r.preview));
      return [...current, ...accepted.slice(0, room)];
    });
    setError(problems.length ? problems.join(". ") : null);
  }, [maxRefs]);

  const removeRef = (id: string) =>
    setRefs((current) => {
      current.filter((r) => r.id === id).forEach((r) => URL.revokeObjectURL(r.preview));
      return current.filter((r) => r.id !== id);
    });

  useEffect(() => {
    if (!generating) return;
    setSeconds(0);
    const id = window.setInterval(() => setSeconds((s) => s + 1), 1000);
    return () => window.clearInterval(id);
  }, [generating]);

  const set = <K extends keyof Settings>(key: K, value: Settings[K]) => setSettings((s) => ({ ...s, [key]: value }));

  const generate = useCallback(async () => {
    const text = prompt.trim();
    if (!text || generating) return;
    setError(null);
    setGenerating(true);
    const controller = new AbortController();
    abortRef.current = controller;
    try {
      const images = await apiGenerateImages({ ...settings, prompt: text }, refs.map((r) => r.file), controller.signal);
      const now = Date.now();
      setHistory((h) => [...images.map((i) => ({ url: i.url, prompt: text, model: settings.model, createdAt: now })), ...h].slice(0, MAX_HISTORY));
    } catch (e) {
      if (controller.signal.aborted) return;
      setError(e instanceof ApiError || e instanceof Error ? e.message : "Gagal membuat gambar.");
    } finally {
      setGenerating(false);
      abortRef.current = null;
    }
  }, [prompt, generating, settings, refs]);

  const ready = config?.enabled && settings.model;

  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <div className="mx-auto grid max-w-6xl gap-6 p-4 md:p-6 lg:grid-cols-[360px_minmax(0,1fr)]">
        <form
          onSubmit={(e) => { e.preventDefault(); void generate(); }}
          className="h-fit space-y-4 rounded-xl border bg-card p-4"
          aria-label="Pengaturan generator gambar"
        >
          <div className="flex items-center gap-2">
            <div className="flex size-8 items-center justify-center rounded-lg bg-primary text-primary-foreground"><Wand2 className="size-4" /></div>
            <h2 className="text-sm font-semibold">Image Generator</h2>
          </div>

          {configError && <p role="alert" className="rounded-lg border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive">{configError}</p>}
          {config && !config.enabled && (
            <p role="status" className="rounded-lg border bg-muted/50 px-3 py-2 text-sm text-muted-foreground">
              Generator gambar belum dikonfigurasi. Isi <code>IMAGE_API_URL</code> dan <code>IMAGE_API_KEY</code> di <code>.env</code> backend, lalu restart backend.
            </p>
          )}

          <Field label="Deskripsi gambar">
            <textarea
              value={prompt}
              onChange={(e) => setPrompt(e.target.value)}
              onKeyDown={(e) => { if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) { e.preventDefault(); void generate(); } }}
              onPaste={(e) => {
                const files = Array.from(e.clipboardData.files).filter((f) => f.type.startsWith("image/"));
                if (files.length > 0 && !generating) { e.preventDefault(); addFiles(files); }
              }}
              maxLength={MAX_PROMPT}
              rows={6}
              placeholder="Contoh: kucing lucu memakai topi, ilustrasi cat air"
              disabled={generating}
              className="w-full resize-y rounded-lg border bg-background px-3 py-2 text-sm outline-none placeholder:text-muted-foreground focus-visible:ring-1 focus-visible:ring-ring disabled:opacity-60"
            />
            <span className="block text-right text-[11px] text-muted-foreground">{prompt.length}/{MAX_PROMPT} · Ctrl/⌘ + Enter untuk membuat</span>
          </Field>

          <div className="space-y-1.5">
            <span className="text-xs font-medium text-muted-foreground">Gambar referensi (opsional)</span>
            <div
              onDragOver={(e) => { e.preventDefault(); setDragging(true); }}
              onDragLeave={() => setDragging(false)}
              onDrop={(e) => { e.preventDefault(); setDragging(false); if (!generating) addFiles(e.dataTransfer.files); }}
              className={`rounded-lg border border-dashed p-3 transition-colors ${dragging ? "border-primary bg-accent/50" : "bg-background"}`}
            >
              {refs.length > 0 && (
                <ul className="mb-3 flex flex-wrap gap-2">
                  {refs.map((r) => (
                    <li key={r.id} className="group relative">
                      <img src={r.preview} alt={`Referensi ${r.file.name}`} className="size-16 rounded-md border object-cover" />
                      <button
                        type="button"
                        onClick={() => removeRef(r.id)}
                        disabled={generating}
                        aria-label={`Hapus referensi ${r.file.name}`}
                        className="absolute -right-1.5 -top-1.5 flex size-5 items-center justify-center rounded-full border bg-background text-foreground shadow-sm hover:bg-accent disabled:opacity-50"
                      ><X className="size-3" /></button>
                    </li>
                  ))}
                </ul>
              )}
              <input
                ref={fileInputRef}
                type="file"
                accept="image/png,image/jpeg,image/webp"
                multiple
                className="sr-only"
                tabIndex={-1}
                onChange={(e) => { if (e.target.files) addFiles(e.target.files); e.target.value = ""; }}
              />
              <Button type="button" variant="outline" size="sm" className="gap-1.5" disabled={generating || refs.length >= maxRefs} onClick={() => fileInputRef.current?.click()}>
                <ImagePlus className="size-4" />Pilih gambar
              </Button>
              <p className="mt-2 text-[11px] text-muted-foreground">Seret gambar ke sini, atau tempel dengan Ctrl/⌘ + V di kolom deskripsi. Maksimal {maxRefs} gambar, 10 MB per gambar (PNG, JPEG, WebP).</p>
            </div>
          </div>

          <Field label="Model">
            <select value={settings.model} onChange={(e) => set("model", e.target.value)} disabled={generating || !config?.models.length} className={fieldClass}>
              {(config?.models ?? []).map((m) => <option key={m} value={m}>{m}</option>)}
            </select>
          </Field>

          <div className="grid grid-cols-2 gap-3">
            <Field label="Jumlah">
              <Select value={String(settings.n)} onChange={(v) => set("n", Number(v))} options={[["1", "1"], ["2", "2"], ["3", "3"], ["4", "4"]]} disabled={generating} />
            </Field>
            <Field label="Format">
              <Select value={settings.outputFormat} onChange={(v) => set("outputFormat", v)} options={FORMATS} disabled={generating} />
            </Field>
          </div>
          <Field label="Ukuran"><Select value={settings.size} onChange={(v) => set("size", v)} options={SIZES} disabled={generating} /></Field>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Kualitas"><Select value={settings.quality} onChange={(v) => set("quality", v)} options={QUALITIES} disabled={generating} /></Field>
            <Field label="Latar"><Select value={settings.background} onChange={(v) => set("background", v)} options={BACKGROUNDS} disabled={generating} /></Field>
          </div>

          <div className="space-y-2">
            <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={settings.promptExtend} onChange={(e) => set("promptExtend", e.target.checked)} disabled={generating} />Perluas deskripsi otomatis</label>
            <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={settings.watermark} onChange={(e) => set("watermark", e.target.checked)} disabled={generating} />Beri watermark</label>
          </div>

          {error && <p role="alert" className="rounded-lg border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive">{error}</p>}

          {generating ? (
            <Button type="button" variant="outline" className="w-full" onClick={() => abortRef.current?.abort()}><X />Batalkan ({seconds} dtk)</Button>
          ) : (
            <Button type="submit" className="w-full" disabled={!ready || !prompt.trim()}><Sparkles />{refs.length > 0 ? "Buat dari referensi" : "Buat gambar"}</Button>
          )}
        </form>

        <section aria-label="Hasil gambar" className="min-w-0">
          <div className="mb-3 flex items-center justify-between">
            <h2 className="text-sm font-semibold">Hasil{history.length > 0 && <span className="ml-2 text-xs font-normal text-muted-foreground">{history.length} gambar terakhir</span>}</h2>
            {history.length > 0 && !generating && (
              <Button type="button" variant="ghost" size="sm" className="gap-1.5 text-xs" onClick={() => setHistory([])}><Trash2 className="size-3.5" />Bersihkan daftar</Button>
            )}
          </div>

          {generating && (
            <div role="status" className="mb-4 grid gap-4 sm:grid-cols-2">
              {Array.from({ length: settings.n }, (_, i) => (
                <div key={i} className="flex aspect-square animate-pulse items-center justify-center rounded-xl border bg-muted/40 text-sm text-muted-foreground">
                  <Loader2 className="mr-2 size-4 animate-spin" />Membuat gambar… {seconds} dtk
                </div>
              ))}
            </div>
          )}

          {history.length === 0 && !generating ? (
            <div className="flex min-h-64 flex-col items-center justify-center gap-2 rounded-xl border border-dashed text-center text-sm text-muted-foreground">
              <ImageIcon className="size-8 opacity-50" />
              <p>Gambar yang Anda buat akan muncul di sini.</p>
            </div>
          ) : (
            <div className="grid gap-4 sm:grid-cols-2">
              {history.map((item) => (
                <figure key={item.url} className="min-w-0 space-y-1.5">
                  <ChatImage src={item.url} alt={item.prompt} className="max-h-[28rem]" />
                  <figcaption className="flex items-start justify-between gap-2 text-xs text-muted-foreground">
                    <span className="line-clamp-2" title={item.prompt}>{item.prompt}</span>
                    <button type="button" className="shrink-0 underline underline-offset-2 hover:text-foreground" onClick={() => setPrompt(item.prompt)}>Pakai prompt</button>
                  </figcaption>
                </figure>
              ))}
            </div>
          )}
        </section>
      </div>
    </div>
  );
}
