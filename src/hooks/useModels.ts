import { useCallback, useEffect, useState } from "react";
import { listModels } from "@/lib/agent";

const MODEL_KEY = "ai-chat-model";

function storedModel(): string | null {
  try {
    return localStorage.getItem(MODEL_KEY);
  } catch {
    return null;
  }
}

export function useModels() {
  const isDemo = !import.meta.env.VITE_OLLAMA_BASE_URL?.trim();
  const [models, setModels] = useState<string[]>([]);
  const [selectedModel, setSelectedModel] = useState<string | null>(
    () => (isDemo ? null : storedModel()),
  );
  const [isLoading, setIsLoading] = useState(!isDemo);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    if (isDemo) return;

    setIsLoading(true);
    setError(null);
    try {
      const loadedModels = await listModels();
      setModels(loadedModels);
      setSelectedModel((current) =>
        current && loadedModels.includes(current)
          ? current
          : (loadedModels[0] ?? null),
      );
    } catch {
      setError("Gagal memuat daftar model. Coba lagi.");
    } finally {
      setIsLoading(false);
    }
  }, [isDemo]);

  useEffect(() => {
    void reload();
  }, [reload]);

  useEffect(() => {
    try {
      if (selectedModel) localStorage.setItem(MODEL_KEY, selectedModel);
      else localStorage.removeItem(MODEL_KEY);
    } catch {
      // Storage can be unavailable; model selection remains in memory.
    }
  }, [selectedModel]);

  return { models, selectedModel, setSelectedModel, isLoading, error, reload, isDemo };
}
