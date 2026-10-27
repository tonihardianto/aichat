import { useState } from "react";
import { Button } from "@/components/ui/button";

type AuthModalProps = {
  open: boolean;
  onClose: () => void;
  onLogin: (email: string, pass: string) => Promise<any>;
  onRegister: (email: string, pass: string) => Promise<any>;
};

export function AuthModal({ open, onClose, onLogin, onRegister }: AuthModalProps) {
  const [isRegister, setIsRegister] = useState(false);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  if (!open) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    if (!email.trim() || !password.trim()) {
      setError("Email dan password wajib diisi");
      return;
    }
    if (password.length < 12) {
      setError("Password minimal 12 karakter");
      return;
    }

    setLoading(true);
    try {
      if (isRegister) {
        await onRegister(email, password);
      } else {
        await onLogin(email, password);
      }
      onClose();
    } catch (err: any) {
      setError(err.message || "Gagal memproses autentikasi");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-xs">
      <div className="w-full max-w-sm rounded-xl border bg-card p-6 shadow-xl">
        <h2 className="text-lg font-semibold tracking-tight">
          {isRegister ? "Daftar Akun Baru" : "Masuk ke AI Chat"}
        </h2>
        <p className="mt-1 text-xs text-muted-foreground">
          {isRegister
            ? "Buat akun untuk menyimpan percakapan secara aman di server."
            : "Masuk untuk mengakses riwayat percakapan Anda."}
        </p>

        {error && (
          <div role="alert" className="mt-3 rounded-md bg-destructive/15 p-2.5 text-xs text-destructive">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="mt-4 space-y-3">
          <div>
            <label className="text-xs font-medium text-foreground">Email</label>
            <input
              type="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="nama@email.com"
              className="mt-1 w-full rounded-md border bg-background px-3 py-2 text-sm outline-none focus:border-primary"
            />
          </div>
          <div>
            <label className="text-xs font-medium text-foreground">Password (minimal 12 karakter)</label>
            <input
              type="password"
              required
              minLength={12}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="••••••••••••"
              className="mt-1 w-full rounded-md border bg-background px-3 py-2 text-sm outline-none focus:border-primary"
            />
          </div>
          <Button type="submit" disabled={loading} className="w-full mt-2">
            {loading ? "Memproses..." : isRegister ? "Daftar" : "Masuk"}
          </Button>
        </form>

        <div className="mt-4 flex items-center justify-between border-t pt-3 text-xs">
          <button
            type="button"
            onClick={() => {
              setIsRegister(!isRegister);
              setError(null);
            }}
            className="text-primary hover:underline"
          >
            {isRegister ? "Sudah punya akun? Masuk" : "Belum punya akun? Daftar"}
          </button>
          <button type="button" onClick={onClose} className="text-muted-foreground hover:text-foreground">
            Tutup
          </button>
        </div>
      </div>
    </div>
  );
}
