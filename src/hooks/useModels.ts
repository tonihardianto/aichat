import { useCallback, useEffect, useRef, useState } from "react";
import { apiListGuestModels, apiListModels } from "@/lib/api";
import { listModels as fallbackListModels } from "@/lib/agent";

const MODEL_KEY = "ai-chat-model";

function storedModel(): string | null {
  try {
    return localStorage.getItem(MODEL_KEY);
  } catch {
    return null;
  }
}

export function useModels(isAuthenticated = true) {
  const [models, setModels] = useState<string[]>([]);
  const [selectedModel, setSelectedModel] = useState<string | null>(() => storedModel());
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isDemo, setIsDemo] = useState(false);
  const requestRef = useRef(0);

  const reload = useCallback(async () => {
    const request = ++requestRef.current;
    setIsLoading(true);
    setError(null);
    try {
      let loadedModels: string[] = [];
      try {
        loadedModels = isAuthenticated ? await apiListModels() : await apiListGuestModels();
      } catch {
        loadedModels = isAuthenticated ? await fallbackListModels() : [];
      }

      if (request !== requestRef.current) return;
      if (loadedModels && loadedModels.length > 0) {
        setModels(loadedModels);
        setIsDemo(false);
        setError(null);
        setSelectedModel((current) =>
          current && loadedModels.includes(current)
            ? current
            : (loadedModels[0] ?? null)
        );
      } else {
        setModels([]);
        setSelectedModel(null);
        setIsDemo(isAuthenticated ? true : false);
        if (!isAuthenticated) setError("Model tamu belum dikonfigurasi.");
      }
    } catch {
      if (request !== requestRef.current) return;
      setError("Gagal memuat daftar model. Coba lagi.");
    } finally {
      if (request === requestRef.current) setIsLoading(false);
    }
  }, [isAuthenticated]);

  useEffect(() => {
    try {
      if (isAuthenticated && selectedModel) localStorage.setItem(MODEL_KEY, selectedModel);
      else localStorage.removeItem(MODEL_KEY);
    } catch {
      // Storage can be unavailable; model selection remains in memory.
    }
  }, [selectedModel, isAuthenticated]);

  return { models, selectedModel, setSelectedModel, isLoading, error, reload, isDemo };
}
