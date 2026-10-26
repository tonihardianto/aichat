# Desain model Ollama dan gambar

## Tujuan

Menghubungkan chat ke server Ollama OpenAI-compatible, memberi pilihan model, dan mendukung lampiran gambar untuk model vision.

## Alur data

- `VITE_OLLAMA_BASE_URL` menunjuk base URL OpenAI-compatible, misalnya `http://10.20.30.35:11434/v1`.
- `lib/agent.ts` mengambil model dari `${baseUrl}/models`, lalu mengirim chat ke `${baseUrl}/chat/completions` dengan `model`, `messages`, dan `stream: true`.
- Model aktif tersimpan di `localStorage`. Daftar model dimuat saat aplikasi mulai dan bisa dimuat ulang.
- Pesan menyimpan lampiran gambar sebagai data URL, lalu adapter mengubah pesan pengguna bergambar menjadi OpenAI content parts: teks dan `image_url`.

## UI

- Header menampilkan model selector dan tombol muat ulang daftar model.
- Composer menyediakan tombol unggah, thumbnail, dan hapus lampiran.
- Hanya satu gambar per pesan; menerima JPEG, PNG, atau WebP maksimal 5 MB.
- Karena endpoint `/models` tidak menyediakan capability vision, aplikasi tidak menebak dukungan model. Pengguna dapat melampirkan gambar; jika endpoint menolaknya, error agent tampil sebagai state retry yang ada.

## Error dan batasan

- Bila base URL kosong, aplikasi tetap memakai demo mode dan selector menampilkan status demo.
- Gagal memuat model tidak menghalangi chat teks; UI menunjukkan error dan tombol muat ulang.
- File invalid atau terlalu besar ditolak di composer dengan pesan Indonesia.
- Base64 disimpan lokal bersama percakapan; gambar besar dibatasi untuk mencegah localStorage penuh.

## Verifikasi

- Test helper membentuk endpoint API dan content parts pesan bergambar.
- Jalankan `node --test` dan `npm run build`.

## Asumsi

- Server menerima format OpenAI `/chat/completions` dengan streaming SSE.
- Server vision, jika nanti dipasang, menerima `image_url` data URL.
