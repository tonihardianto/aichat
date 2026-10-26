import { useState } from "react";
import { MoreHorizontal, Pencil, Plus, Trash2 } from "lucide-react";
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
  conversations: Conversation[]; activeId: string; onSelect: (id: string) => void;
  onNew: () => void; onRename: (id: string, title: string) => void; onDelete: (id: string) => void;
};

export function Sidebar({ conversations, activeId, onSelect, onNew, onRename, onDelete }: SidebarProps) {
  const [renaming, setRenaming] = useState<string | null>(null);
  return <aside className="flex h-full w-full flex-col bg-card p-3">
    <Button className="w-full justify-start gap-2" variant="outline" onClick={onNew}><Plus />Percakapan baru</Button>
    <h2 className="px-2 pb-2 pt-6 text-xs font-semibold uppercase tracking-wide text-muted-foreground">Riwayat</h2>
    <nav aria-label="Riwayat percakapan" className="min-h-0 flex-1 space-y-1 overflow-y-auto">
      {conversations.map((conversation) => <div key={conversation.id} className={`group flex items-center rounded-md ${activeId === conversation.id ? "bg-accent" : "hover:bg-accent/60"}`}>
        {renaming === conversation.id ? <form className="flex min-w-0 flex-1 p-1" onSubmit={(event) => { event.preventDefault(); const input = new FormData(event.currentTarget).get("title"); onRename(conversation.id, String(input ?? "")); setRenaming(null); }}>
          <input autoFocus name="title" aria-label="Nama percakapan" defaultValue={conversation.title} className="min-w-0 flex-1 rounded border bg-background px-2 py-1 text-sm" onBlur={() => setRenaming(null)} onKeyDown={(event) => { if (event.key === "Escape") setRenaming(null); }} />
        </form> : <button onClick={() => onSelect(conversation.id)} aria-current={activeId === conversation.id ? "page" : undefined} className="min-w-0 flex-1 truncate px-3 py-2 text-left text-sm">{conversation.title}</button>}
        <DropdownMenu><DropdownMenuTrigger asChild><Button variant="ghost" size="icon" aria-label={`Aksi ${conversation.title}`} className="mr-1 size-8 shrink-0 opacity-100 sm:opacity-0 sm:group-hover:opacity-100 sm:focus-visible:opacity-100"><MoreHorizontal /></Button></DropdownMenuTrigger><DropdownMenuContent align="end">
          <DropdownMenuItem onSelect={() => setRenaming(conversation.id)}><Pencil />Ubah nama</DropdownMenuItem>
          <AlertDialog><AlertDialogTrigger asChild><button className="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-sm text-destructive outline-none hover:bg-accent"><Trash2 className="size-4" />Hapus</button></AlertDialogTrigger><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Hapus percakapan?</AlertDialogTitle><AlertDialogDescription>Percakapan ini akan dihapus permanen.</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>Batal</AlertDialogCancel><AlertDialogAction onClick={() => onDelete(conversation.id)}>Hapus</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>
        </DropdownMenuContent></DropdownMenu>
      </div>)}
    </nav>
    <div className="border-t px-2 pt-3 text-xs text-muted-foreground">AI Chat</div>
  </aside>;
}
