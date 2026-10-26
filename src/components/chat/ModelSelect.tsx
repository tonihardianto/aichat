import { RefreshCw } from "lucide-react";

type ModelSelectProps = {
  models: string[];
  value: string | null;
  loading: boolean;
  error: string | null;
  isDemo: boolean;
  onChange: (model: string) => void;
  onReload: () => void;
};

export function ModelSelect({
  models,
  value,
  loading,
  error,
  isDemo,
  onChange,
  onReload,
}: ModelSelectProps) {
  if (isDemo) {
    return <span role="status" className="text-xs text-muted-foreground">Mode demo</span>;
  }

  return <div className="flex min-w-0 flex-wrap items-center gap-1">
    <label htmlFor="model" className="sr-only">Model</label>
    <select
      id="model"
      value={value ?? ""}
      disabled={loading || models.length === 0}
      onChange={(event) => onChange(event.target.value)}
      className="h-8 max-w-32 rounded-md border bg-background px-2 text-xs"
    >
      {loading ? <option value="">Memuat model...</option> : models.length === 0 ? <option value="">Model tidak tersedia</option> : models.map((model) => <option key={model} value={model}>{model}</option>)}
    </select>
    <button
      type="button"
      onClick={onReload}
      disabled={loading}
      aria-label="Muat ulang model"
      className="inline-flex size-8 shrink-0 items-center justify-center rounded-md border hover:bg-accent disabled:opacity-50"
    >
      <RefreshCw className={loading ? "size-4 animate-spin" : "size-4"} />
    </button>
    {error && <span role="alert" className="basis-full text-xs text-destructive">{error} <button type="button" onClick={onReload} className="underline">Muat ulang</button></span>}
  </div>;
}
