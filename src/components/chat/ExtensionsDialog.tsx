import { useEffect, useState } from "react";
import { Bot, Plus, Trash2, Wrench } from "lucide-react";
import { Button } from "@/components/ui/button";
import { apiDeleteMemory, apiDeleteSkill, apiDeleteTool, apiListMemories, apiListSkills, apiListTools, apiSaveSkill, apiSaveTool, type ApiMemory, type ApiSkill, type ApiTool } from "@/lib/api";

const schema = `{"type":"object","properties":{}}`;

export function ExtensionsDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [tools, setTools] = useState<ApiTool[]>([]); const [skills, setSkills] = useState<ApiSkill[]>([]); const [memories, setMemories] = useState<ApiMemory[]>([]); const [error, setError] = useState("");
  const [tool, setTool] = useState({ name: "", description: "", inputSchema: schema, webhookUrl: "", enabled: true });
  const [skill, setSkill] = useState({ name: "", description: "", instructions: "", enabled: true });
  const load = () => Promise.all([apiListTools(), apiListSkills(), apiListMemories()]).then(([t, s, m]) => { setTools(t ?? []); setSkills(s ?? []); setMemories(m ?? []); }).catch((e: Error) => setError(e.message));
  useEffect(() => { if (open) { setError(""); void load(); } }, [open]);
  if (!open) return null;
  const saveTool = async (event: React.FormEvent) => { event.preventDefault(); try { await apiSaveTool(tool); setTool({ name: "", description: "", inputSchema: schema, webhookUrl: "", enabled: true }); await load(); } catch (e) { setError(e instanceof Error ? e.message : "Gagal menyimpan tool"); } };
  const saveSkill = async (event: React.FormEvent) => { event.preventDefault(); try { await apiSaveSkill(skill); setSkill({ name: "", description: "", instructions: "", enabled: true }); await load(); } catch (e) { setError(e instanceof Error ? e.message : "Gagal menyimpan skill"); } };
  return <div role="dialog" aria-modal="true" aria-label="Tools dan skills" className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4" onMouseDown={onClose}>
    <div className="max-h-[90dvh] w-full max-w-3xl overflow-y-auto rounded-xl border bg-background p-5 shadow-xl" onMouseDown={(e) => e.stopPropagation()}>
      <div className="mb-5 flex items-center justify-between"><div><h2 className="text-lg font-semibold">Tools & Skills</h2><p className="text-sm text-muted-foreground">`web_search` dan `web_fetch` aktif bawaan.</p></div><Button variant="ghost" onClick={onClose}>Tutup</Button></div>
      {error && <p role="alert" className="mb-4 rounded border border-destructive/30 p-3 text-sm text-destructive">{error}</p>}
      <div className="grid gap-6 md:grid-cols-2">
        <section><h3 className="mb-3 flex items-center gap-2 font-medium"><Wrench className="size-4" />Tool webhook</h3><form onSubmit={saveTool} className="space-y-2">
          <input required placeholder="nama_tool" value={tool.name} onChange={(e) => setTool({ ...tool, name: e.target.value })} className="w-full rounded border bg-background p-2 text-sm" />
          <input required placeholder="Deskripsi" value={tool.description} onChange={(e) => setTool({ ...tool, description: e.target.value })} className="w-full rounded border bg-background p-2 text-sm" />
          <input required type="url" placeholder="https://api.example.com/tool" value={tool.webhookUrl} onChange={(e) => setTool({ ...tool, webhookUrl: e.target.value })} className="w-full rounded border bg-background p-2 text-sm" />
          <textarea required aria-label="JSON Schema input" value={tool.inputSchema} onChange={(e) => setTool({ ...tool, inputSchema: e.target.value })} className="min-h-24 w-full rounded border bg-background p-2 font-mono text-xs" />
          <Button size="sm" type="submit"><Plus />Tambah tool</Button></form>
          <ExtensionList entries={tools} onDelete={async (id) => { await apiDeleteTool(id); await load(); }} />
        </section>
        <section><h3 className="mb-3 flex items-center gap-2 font-medium"><Bot className="size-4" />Skill instruksi</h3><form onSubmit={saveSkill} className="space-y-2">
          <input required placeholder="Nama skill" value={skill.name} onChange={(e) => setSkill({ ...skill, name: e.target.value })} className="w-full rounded border bg-background p-2 text-sm" />
          <input required placeholder="Deskripsi" value={skill.description} onChange={(e) => setSkill({ ...skill, description: e.target.value })} className="w-full rounded border bg-background p-2 text-sm" />
          <textarea required placeholder="Instruksi yang diberikan ke AI" value={skill.instructions} onChange={(e) => setSkill({ ...skill, instructions: e.target.value })} className="min-h-32 w-full rounded border bg-background p-2 text-sm" />
          <Button size="sm" type="submit"><Plus />Tambah skill</Button></form>
          <ExtensionList entries={skills} onDelete={async (id) => { await apiDeleteSkill(id); await load(); }} />
        </section>
      </div>
      <section className="mt-6"><h3 className="font-medium">Memori AI</h3><p className="text-sm text-muted-foreground">Fakta tentang Anda yang disimpan AI lewat tool `remember`.</p>
        <ExtensionList entries={memories.map((m) => ({ id: m.id, name: m.content }))} onDelete={async (id) => { await apiDeleteMemory(id); await load(); }} />
      </section>
    </div>
  </div>;
}

function ExtensionList({ entries, onDelete }: { entries: Array<{ id: string; name: string }> | null; onDelete: (id: string) => Promise<void> }) {
  return <ul className="mt-4 space-y-2">{(entries ?? []).map((item) => <li key={item.id} className="flex items-center gap-2 rounded border px-3 py-2 text-sm"><span className="min-w-0 flex-1 truncate">{item.name}</span><Button size="icon" variant="ghost" aria-label={`Hapus ${item.name}`} onClick={() => void onDelete(item.id)}><Trash2 className="size-4 text-destructive" /></Button></li>)}</ul>;
}
