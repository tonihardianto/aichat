# Chat AI

Antarmuka chat AI dengan riwayat percakapan, streaming, markdown, mode terang/gelap, dan penyimpanan lokal.

## Jalankan

```bash
npm install
npm run dev
```

Buka URL yang dicetak Vite. Untuk memeriksa hasil produksi:

```bash
npm run build
npm run preview
```

## Menghubungkan agen asli

Buat `.env.local` di akar proyek:

```env
VITE_AGENT_URL=https://agent.example.com/chat
```

Jalankan ulang `npm run dev` setelah mengubah env. Vite menanamkan nilai ini saat build, jadi build produksi juga perlu env yang benar.

Jika `VITE_AGENT_URL` kosong atau tidak diset, aplikasi memakai mode demo: balasan dipalsukan dan di-stream bertahap tanpa memanggil server.

## Kontrak transport

Aplikasi mengirim `POST` ke `VITE_AGENT_URL` dengan:

```http
Content-Type: application/json
```

Body berisi seluruh riwayat percakapan yang sedang aktif:

```json
{
  "messages": [
    { "role": "user", "content": "Halo" },
    { "role": "assistant", "content": "Hai, ada yang bisa dibantu?" },
    { "role": "user", "content": "Jelaskan streaming." }
  ]
}
```

Setiap `role` hanya `user` atau `assistant`. Kirim riwayat lengkap setiap permintaan, bukan hanya pesan terakhir.

### Respons yang diterima

Balasan non-2xx dianggap error. Isi respons harus stream; body kosong juga error.

**Teks biasa** — `Content-Type` selain `text/event-stream`, misalnya `text/plain`. Setiap potongan body langsung ditampilkan:

```text
Halo, saya agen.
```

**SSE** — `Content-Type: text/event-stream`. Setiap event membawa satu token:

```text
data: {"token":"Halo"}

data: {"token":" dunia"}

data: [DONE]
```

`[DONE]` menutup stream. Field token yang diterima: `token`, `delta`, `content`, atau `text`. String JSON langsung (`data: "Halo"`) dan data mentah (`data: Halo`) juga diterima. Respons tanpa `[DONE]` tetap dianggap selesai saat koneksi ditutup.

## Data lokal

Percakapan disimpan di `localStorage` browser dengan key `ai-chat-conversations-v1`. Tidak ada database atau sinkronisasi server; menghapus data situs akan menghapus riwayat.
