import { useEffect, useRef, useState } from "react";

/* ───────────── 1. Hubungkan ke AI agent Anda di sini ─────────────
   Isi AGENT_URL dengan endpoint agent Anda (mis. server Go).
   Kosongkan untuk mode demo (balasan simulasi).

   Request  : POST { messages: [{role, content}], conversation_id }
   Response : stream teks biasa, ATAU SSE ("data: {...}") dengan field
              token / delta / content / text.
*/
const AGENT_URL = "";

type Role = "user" | "assistant";
type Msg = { id: string; role: Role; content: string };
type Chat = { id: string; title: string; messages: Msg[] };

const uid = () => Math.random().toString(36).slice(2, 10);

async function askAgent(
  chat: Chat,
  signal: AbortSignal,
  onToken: (t: string) => void
) {
  if (!AGENT_URL) {
    const last = chat.messages[chat.messages.length - 1]?.content ?? "";
    const demo = `Ini balasan demo untuk: "${last}". Isi AGENT_URL di ChatPage.tsx agar terhubung ke AI agent Anda.`;
    for (const w of demo.split(" ")) {
      if (signal.aborted) return;
      await new Promise((r) => setTimeout(r, 40));
      onToken(w + " ");
    }
    return;
  }

  const res = await fetch(AGENT_URL, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      conversation_id: chat.id,
      messages: chat.messages.map(({ role, content }) => ({ role, content })),
    }),
    signal,
  });
  if (!res.ok || !res.body) throw new Error(`Agent menjawab ${res.status}`);

  const reader = res.body.getReader();
  const dec = new TextDecoder();
  let buf = "";
  const isSSE = (res.headers.get("content-type") ?? "").includes("event-stream");

  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    const chunk = dec.decode(value, { stream: true });
    if (!isSSE) {
      onToken(chunk);
      continue;
    }
    buf += chunk;
    const lines = buf.split("\n");
    buf = lines.pop() ?? "";
    for (const line of lines) {
      if (!line.startsWith("data:")) continue;
      const data = line.slice(5).trim();
      if (!data || data === "[DONE]") continue;
      try {
        const j = JSON.parse(data);
        onToken(j.token ?? j.delta ?? j.content ?? j.text ?? "");
      } catch {
        onToken(data);
      }
    }
  }
}

/* ───────────── 2. Komponen ───────────── */
export default function ChatPage() {
  const [chats, setChats] = useState<Chat[]>([
    { id: uid(), title: "Percakapan baru", messages: [] },
  ]);
  const [activeId, setActiveId] = useState(chats[0].id);
  const [input, setInput] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [menu, setMenu] = useState(false);
  const abortRef = useRef<AbortController | null>(null);
  const endRef = useRef<HTMLDivElement>(null);
  const taRef = useRef<HTMLTextAreaElement>(null);

  const chat = chats.find((c) => c.id === activeId)!;

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: "smooth", block: "end" });
  }, [chat.messages]);

  useEffect(() => {
    const ta = taRef.current;
    if (!ta) return;
    ta.style.height = "auto";
    ta.style.height = Math.min(ta.scrollHeight, 200) + "px";
  }, [input]);

  const patch = (id: string, fn: (c: Chat) => Chat) =>
    setChats((cs) => cs.map((c) => (c.id === id ? fn(c) : c)));

  const newChat = () => {
    const c = { id: uid(), title: "Percakapan baru", messages: [] };
    setChats((cs) => [c, ...cs]);
    setActiveId(c.id);
    setMenu(false);
  };

  const send = async () => {
    const text = input.trim();
    if (!text || busy) return;
    setInput("");
    setError("");
    const id = chat.id;
    const userMsg: Msg = { id: uid(), role: "user", content: text };
    const botMsg: Msg = { id: uid(), role: "assistant", content: "" };
    const next: Chat = { ...chat, messages: [...chat.messages, userMsg] };
    patch(id, (c) => ({
      ...c,
      title: c.messages.length === 0 ? text.slice(0, 32) : c.title,
      messages: [...next.messages, botMsg],
    }));

    const ctrl = new AbortController();
    abortRef.current = ctrl;
    setBusy(true);
    try {
      await askAgent(next, ctrl.signal, (t) =>
        patch(id, (c) => ({
          ...c,
          messages: c.messages.map((m) =>
            m.id === botMsg.id ? { ...m, content: m.content + t } : m
          ),
        }))
      );
    } catch (e) {
      if ((e as Error).name !== "AbortError")
        setError("Gagal menghubungi agent. Periksa AGENT_URL dan koneksi Anda.");
    } finally {
      setBusy(false);
      abortRef.current = null;
    }
  };

  return (
    <div className="cp">
      <style>{css}</style>

      <aside className={"cp-side" + (menu ? " open" : "")}>
        <button className="cp-new" onClick={newChat}>+ Percakapan baru</button>
        <nav>
          {chats.map((c) => (
            <button
              key={c.id}
              className={"cp-item" + (c.id === activeId ? " on" : "")}
              onClick={() => { setActiveId(c.id); setMenu(false); }}
            >
              {c.title}
            </button>
          ))}
        </nav>
      </aside>

      <main className="cp-main">
        <header className="cp-top">
          <button className="cp-burger" onClick={() => setMenu(!menu)} aria-label="Menu">☰</button>
          <span>{chat.title}</span>
        </header>

        <div className="cp-scroll">
          <div className="cp-col">
            {chat.messages.length === 0 && (
              <div className="cp-empty">
                <h1>Apa yang ingin Anda kerjakan?</h1>
                <p>Ketik tugas untuk AI agent Anda di bawah.</p>
              </div>
            )}
            {chat.messages.map((m) => (
              <div key={m.id} className={"cp-msg " + m.role}>
                <div className="cp-bubble">
                  {m.content || (busy && <span className="cp-dots"><i /><i /><i /></span>)}
                </div>
              </div>
            ))}
            {error && <div className="cp-err">{error}</div>}
            <div ref={endRef} />
          </div>
        </div>

        <div className="cp-compose">
          <div className="cp-box">
            <textarea
              ref={taRef}
              rows={1}
              value={input}
              placeholder="Kirim pesan ke agent…"
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !e.shiftKey) {
                  e.preventDefault();
                  send();
                }
              }}
            />
            {busy ? (
              <button className="cp-btn stop" onClick={() => abortRef.current?.abort()}>Hentikan</button>
            ) : (
              <button className="cp-btn" onClick={send} disabled={!input.trim()}>Kirim</button>
            )}
          </div>
          <small>Enter untuk kirim, Shift+Enter untuk baris baru.</small>
        </div>
      </main>
    </div>
  );
}

const css = `
.cp{--bg:#f6f7f9;--panel:#eceef2;--ink:#171a21;--mute:#667085;--line:#d9dde5;--me:#1f3a5f;--meink:#fff;--bot:#fff;--acc:#1f3a5f;
 display:flex;height:100vh;height:100dvh;background:var(--bg);color:var(--ink);font:15px/1.6 "Segoe UI",system-ui,sans-serif}
@media(prefers-color-scheme:dark){.cp{--bg:#12151b;--panel:#191d25;--ink:#e8eaee;--mute:#8b93a3;--line:#2a2f3a;--me:#3b5f94;--bot:#1c212b;--acc:#6f95cf}}
.cp *{box-sizing:border-box}
.cp button{font:inherit;cursor:pointer}
.cp button:focus-visible,.cp textarea:focus-visible{outline:2px solid var(--acc);outline-offset:2px}
.cp-side{width:260px;background:var(--panel);border-right:1px solid var(--line);padding:12px;display:flex;flex-direction:column;gap:12px;overflow-y:auto}
.cp-new{padding:10px 12px;border:1px solid var(--line);border-radius:8px;background:var(--bg);color:var(--ink);text-align:left}
.cp-new:hover{border-color:var(--acc)}
.cp-side nav{display:flex;flex-direction:column;gap:2px}
.cp-item{padding:8px 10px;border:0;border-radius:6px;background:transparent;color:var(--ink);text-align:left;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.cp-item:hover{background:var(--line)}
.cp-item.on{background:var(--line);font-weight:600}
.cp-main{flex:1;display:flex;flex-direction:column;min-width:0}
.cp-top{display:none;align-items:center;gap:10px;padding:10px 14px;border-bottom:1px solid var(--line)}
.cp-burger{border:0;background:none;color:var(--ink);font-size:20px}
.cp-scroll{flex:1;overflow-y:auto}
.cp-col{max-width:760px;margin:0 auto;padding:24px 16px;display:flex;flex-direction:column;gap:16px}
.cp-empty{margin:18vh 0 0;text-align:center}
.cp-empty h1{font-size:26px;margin:0 0 6px}
.cp-empty p{margin:0;color:var(--mute)}
.cp-msg{display:flex}
.cp-msg.user{justify-content:flex-end}
.cp-bubble{max-width:85%;padding:10px 14px;border-radius:14px;white-space:pre-wrap;overflow-wrap:anywhere}
.cp-msg.user .cp-bubble{background:var(--me);color:var(--meink);border-bottom-right-radius:4px}
.cp-msg.assistant .cp-bubble{background:var(--bot);border:1px solid var(--line);border-bottom-left-radius:4px}
.cp-dots{display:inline-flex;gap:4px;padding:4px 0}
.cp-dots i{width:6px;height:6px;border-radius:50%;background:var(--mute);animation:cpb 1s infinite}
.cp-dots i:nth-child(2){animation-delay:.15s}.cp-dots i:nth-child(3){animation-delay:.3s}
@keyframes cpb{0%,80%,100%{opacity:.3}40%{opacity:1}}
.cp-err{color:#c0392b;font-size:14px}
.cp-compose{padding:8px 16px 14px;max-width:792px;width:100%;margin:0 auto}
.cp-box{display:flex;align-items:flex-end;gap:8px;padding:8px;border:1px solid var(--line);border-radius:14px;background:var(--bot)}
.cp-box:focus-within{border-color:var(--acc)}
.cp-box textarea{flex:1;border:0;outline:0;resize:none;background:transparent;color:var(--ink);font:inherit;padding:6px 8px;max-height:200px}
.cp-btn{padding:8px 16px;border:0;border-radius:10px;background:var(--acc);color:#fff}
.cp-btn:disabled{opacity:.4;cursor:default}
.cp-btn.stop{background:var(--mute)}
.cp-compose small{display:block;margin-top:6px;text-align:center;color:var(--mute);font-size:12px}
@media(max-width:720px){
 .cp-side{position:fixed;inset:0 auto 0 0;z-index:5;transform:translateX(-100%);transition:transform .2s}
 .cp-side.open{transform:none}
 .cp-top{display:flex}
}
@media(prefers-reduced-motion:reduce){.cp *{animation:none!important;transition:none!important}}
`;
