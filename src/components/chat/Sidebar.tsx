import { useState } from "react";
import { LogOut, MoreHorizontal, Pencil, Plus, Trash2, User as UserIcon, Sparkles, Wrench } from "lucide-react";
import type { Conversation } from "@/types/chat";
import { Button } from "@/components/ui/button";
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent,
  AlertDialogDescription, AlertDialogFooter, AlertDialogHeader,
  AlertDialogTitle, AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

type SidebarProps = {
  conversations: Conversation[];
  activeId: string;
  onSelect: (id: string) => void;
  onNew: () => void;
  onRename: (id: string, title: string) => void;
  onDelete: (id: string) => void;
  user?: { email: string } | null;
  onLogout?: () => void;
  onOpenAuth?: () => void;
  onOpenExtensions?: () => void;
};

export function Sidebar({
  conversations,
  activeId,
  onSelect,
  onNew,
  onRename,
  onDelete,
  user,
  onLogout,
  onOpenAuth,
  onOpenExtensions,
}: SidebarProps) {
  const [renaming, setRenaming] = useState<string | null>(null);
  const guestMode = !user;

  return (
    <aside className="flex h-full w-full flex-col bg-card p-3">
      <div className="mb-5 flex items-center gap-2 px-2 py-1.5">
        <div className="flex size-8 items-center justify-center rounded-xl bg-gradient-to-br from-primary to-violet-500 text-primary-foreground shadow-sm">
          <Sparkles className="size-4" />
        </div>
        <div className="min-w-0">
          <p className="truncate text-sm font-semibold">AI Chat</p>
          <p className="text-xs text-muted-foreground">Ollama lokal</p>
        </div>
      </div>
      {!guestMode && <Button className="w-full justify-start gap-2" variant="outline" onClick={onNew}>
        <Plus />Percakapan baru
      </Button>}
      {guestMode ? <p className="px-2 pb-2 pt-6 text-xs text-muted-foreground">Riwayat tamu hanya tersedia selama sesi ini.</p> : <h2 className="px-2 pb-2 pt-6 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">Percakapan</h2>}
      <nav aria-label={guestMode ? "Percakapan sementara" : "Riwayat percakapan"} className="min-h-0 flex-1 space-y-1 overflow-y-auto">
        {conversations.map((conversation) => (
          <div
            key={conversation.id}
            className={`group flex items-center rounded-lg transition-colors ${
              activeId === conversation.id ? "bg-accent font-medium text-accent-foreground" : "text-muted-foreground hover:bg-accent/60 hover:text-foreground"
            }`}
          >
            {!guestMode && renaming === conversation.id ? (
              <form
                className="flex min-w-0 flex-1 p-1"
                onSubmit={(event) => {
                  event.preventDefault();
                  const input = new FormData(event.currentTarget).get("title");
                  onRename(conversation.id, String(input ?? ""));
                  setRenaming(null);
                }}
              >
                <input
                  autoFocus
                  name="title"
                  aria-label="Nama percakapan"
                  defaultValue={conversation.title}
                  className="min-w-0 flex-1 rounded border bg-background px-2 py-1 text-sm"
                  onBlur={() => setRenaming(null)}
                  onKeyDown={(event) => {
                    if (event.key === "Escape") setRenaming(null);
                  }}
                />
              </form>
            ) : (
              <button
                onClick={() => onSelect(conversation.id)}
                aria-current={activeId === conversation.id ? "page" : undefined}
                className="min-w-0 flex-1 truncate px-3 py-2 text-left text-sm"
              >
                {conversation.title}
              </button>
            )}
            {!guestMode && <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={`Aksi ${conversation.title}`}
                  className="mr-1 size-8 shrink-0 opacity-100 sm:opacity-0 sm:group-hover:opacity-100 sm:focus-visible:opacity-100"
                >
                  <MoreHorizontal />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onSelect={() => setRenaming(conversation.id)}>
                  <Pencil />Ubah nama
                </DropdownMenuItem>
                <AlertDialog>
                  <AlertDialogTrigger asChild>
                    <button className="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-sm text-destructive outline-none hover:bg-accent">
                      <Trash2 className="size-4" />Hapus
                    </button>
                  </AlertDialogTrigger>
                  <AlertDialogContent>
                    <AlertDialogHeader>
                      <AlertDialogTitle>Hapus percakapan?</AlertDialogTitle>
                      <AlertDialogDescription>Percakapan ini akan dihapus permanen.</AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                      <AlertDialogCancel>Batal</AlertDialogCancel>
                      <AlertDialogAction onClick={() => onDelete(conversation.id)}>Hapus</AlertDialogAction>
                    </AlertDialogFooter>
                  </AlertDialogContent>
                </AlertDialog>
              </DropdownMenuContent>
            </DropdownMenu>}
          </div>
        ))}
      </nav>
      <div className="border-t px-2 pt-3 text-xs text-muted-foreground flex items-center justify-between">
        {user ? (
          <div className="flex w-full items-center justify-between gap-1">
            <span className="truncate text-xs font-medium text-foreground max-w-[140px]" title={user.email}>
              {user.email}
            </span>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              onClick={onLogout}
              aria-label="Keluar"
              className="size-7"
            >
              <LogOut className="size-3.5" />
            </Button>
            <Button type="button" variant="ghost" size="icon" onClick={onOpenExtensions} aria-label="Tools dan skills" className="size-7"><Wrench className="size-3.5" /></Button>
          </div>
        ) : (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={onOpenAuth}
            className="w-full justify-start gap-1.5 text-xs h-8"
          >
            <UserIcon className="size-3.5" />
            <span>Masuk / Daftar</span>
          </Button>
        )}
      </div>
    </aside>
  );
}
