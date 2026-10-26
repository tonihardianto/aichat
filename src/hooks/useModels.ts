import { useCallback, useEffect, useRef, useState } from "react";
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
  const initialLoadRef = useRef(false);
  const requestRef = useRef(0);

  const reload = useCallback(async () => {
    if (isDemo) return;

    const request = ++requestRef.current;
    setIsLoading(true);
    setError(null);
    try {
      const loadedModels = await listModels();
      if (request !== requestRef.current) return;
      setModels(loadedModels);
      setSelectedModel((current) =>
        current && loadedModels.includes(current)
          ? current
          : (loadedModels[0] ?? null),
      );
    } catch {
      if (request !== requestRef.current) return;
      setError("Gagal memuat daftar model. Coba lagi.");
    } finally {
      if (request === requestRef.current) setIsLoading(false);
    }
  }, [isDemo]);

  useEffect(() => {
    if (initialLoadRef.current) return;
    initialLoadRef.current = true;
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
