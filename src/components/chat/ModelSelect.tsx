import { RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";

type ModelSelectProps = {
  models: string[];
  value: string | null;
  loading: boolean;
  error: string | null;
  isDemo: boolean;
  onChange: (model: string) => void;
  onReload: () => void;
  guestMode?: boolean;
};

export function ModelSelect({
  models,
  value,
  loading,
  error,
  isDemo,
  onChange,
  onReload,
  guestMode = false,
}: ModelSelectProps) {
  if (isDemo) {
    return <span role="status" className="inline-flex items-center rounded-md bg-secondary px-2.5 py-1 text-xs font-medium text-secondary-foreground">Mode demo</span>;
  }

  return <div className="flex min-w-0 flex-wrap items-center gap-1">
    {!guestMode && <label htmlFor="model" className="sr-only">Model</label>}
      {!guestMode && <select
      id="model"
      aria-label="Model Ollama"
      value={value ?? ""}
      disabled={loading || models.length === 0}
      onChange={(event) => onChange(event.target.value)}
      className="h-9 max-w-40 rounded-lg border bg-background px-3 text-xs transition-colors hover:bg-accent/50"
    >
      {loading ? <option value="">Memuat model...</option> : models.length === 0 ? <option value="">Model tidak tersedia</option> : models.map((model) => <option key={model} value={model}>{model}</option>)}
      </select>}
    {!guestMode && <Button
      type="button"
      onClick={onReload}
      disabled={loading}
      aria-label="Muat ulang model"
      variant="ghost"
      size="icon"
      className="size-9 shrink-0"
    >
      <RefreshCw className={loading ? "size-4 animate-spin" : "size-4"} />
    </Button>}
    {error && <span role="alert" className="basis-full text-xs text-destructive">{error} <Button type="button" onClick={onReload} variant="link" size="sm" className="h-auto p-0 text-xs">Muat ulang</Button></span>}
  </div>;
}
