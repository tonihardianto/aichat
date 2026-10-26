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

`VITE_AGENT_URL` adalah fallback lama untuk endpoint chat kustom. Jika `VITE_OLLAMA_BASE_URL` diset, aplikasi memakai Ollama dan mengabaikan `VITE_AGENT_URL`. Mode demo hanya aktif jika kedua variabel kosong: balasan dipalsukan dan di-stream bertahap tanpa memanggil server.

## Menghubungkan Ollama

Ollama menyediakan API yang kompatibel dengan OpenAI. Taruh konfigurasi di `.env.local` pada akar proyek dan set `VITE_OLLAMA_BASE_URL` ke base URL API, termasuk `/v1`:

```env
VITE_OLLAMA_BASE_URL=http://10.20.30.35:11434/v1
```

Jalankan ulang `npm run dev` setelah mengubah env. Ollama dipakai jika variabel ini diset. Jika kosong, aplikasi memakai `VITE_AGENT_URL` bila ada; jika keduanya kosong, aplikasi memakai mode demo.

Saat halaman dibuka, pemilih model memuat `GET ${VITE_OLLAMA_BASE_URL}/models`. Chat mengirim `POST ${VITE_OLLAMA_BASE_URL}/chat/completions` dengan riwayat percakapan dan `stream: true`. Pilih model yang sudah terpasang di server Ollama; jika model tersimpan tidak ada, aplikasi memakai model pertama dari daftar.

### Gambar

Aplikasi tidak mendeteksi kemampuan vision model. Pilih model yang mendukung vision (misalnya `llava`) dan sudah terpasang di server Ollama. Jika model menolak gambar, penolakan dari server tampil sebagai error dengan tombol coba lagi.

Satu pesan dapat membawa satu gambar dengan format JPEG, PNG, atau WebP, ukuran maksimal 5 MiB (5.242.880 byte). Gambar dikirim sebagai data URL di bagian `image_url` pada payload pesan.

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
