import { useCallback, useEffect, useState } from "react";
import {
  apiGetMe,
  apiLogin,
  apiLogout,
  apiRegister,
  type AuthUser,
} from "@/lib/api";

export function useAuth() {
  const [user, setUser] = useState<AuthUser | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const checkAuth = useCallback(async () => {
    setIsLoading(true);
    try {
      const u = await apiGetMe();
      setUser(u);
      setError(null);
    } catch {
      setUser(null);
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    void checkAuth();
  }, [checkAuth]);

  const login = async (email: string, pass: string) => {
    setIsLoading(true);
    setError(null);
    try {
      const u = await apiLogin(email, pass);
      setUser(u);
      return u;
    } catch (err: any) {
      const msg = err.message || "Gagal masuk";
      setError(msg);
      throw err;
    } finally {
      setIsLoading(false);
    }
  };

  const register = async (email: string, pass: string) => {
    setIsLoading(true);
    setError(null);
    try {
      const u = await apiRegister(email, pass);
      setUser(u);
      return u;
    } catch (err: any) {
      const msg = err.message || "Gagal mendaftar";
      setError(msg);
      throw err;
    } finally {
      setIsLoading(false);
    }
  };

  const logout = async () => {
    try {
      await apiLogout();
    } finally {
      setUser(null);
    }
  };

  return {
    user,
    isLoading,
    error,
    login,
    register,
    logout,
    checkAuth,
    clearError: () => setError(null),
  };
}
