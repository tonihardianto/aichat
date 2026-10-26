# Desain Chat AI

## Tujuan

Membangun aplikasi chat AI React, TypeScript, Vite dengan antarmuka Indonesia. Riwayat percakapan disimpan di `localStorage`; jawaban AI mengalir dari adapter tunggal dan mempunyai mode demo saat URL agent kosong.

## Arsitektur

- `App.tsx` menyusun sidebar dan ruang chat.
- `useConversations` mengelola percakapan CRUD serta persistence `localStorage`.
- `useChat` mengirim pesan, menerima token streaming, menghentikan request, membuat ulang jawaban, dan menyediakan retry.
- `lib/agent.ts` mengekspor `streamReply(messages, signal, onToken)`. URL dibaca dari `VITE_AGENT_URL`. Adapter membaca SSE atau respons teks biasa; URL kosong memakai respons demo bertahap.
- Komponen chat: `Sidebar`, `MessageList`, `MessageBubble`, `Composer`.
- Komponen dasar memakai shadcn/ui: Button, Textarea, Sheet, DropdownMenu, AlertDialog, Tooltip.

## Model data

- `Conversation`: `id`, `title`, `messages`, `createdAt`, `updatedAt`.
- `Message`: `id`, `role` (`user` atau `assistant`), `content`, `createdAt`.
- Judul memakai 40 karakter awal pesan pengguna pertama.

## Perilaku UI

- Desktop memakai sidebar tetap; mobile memakai `Sheet` drawer.
- Area kosong menempatkan sapaan dan composer di tengah.
- Balasan assistant dirender sebagai Markdown dengan syntax highlight ringan dan tombol salin setiap blok kode.
- Setiap jawaban assistant dapat disalin atau dibuat ulang.
- Scroll mengikuti token baru hanya bila pengguna berada dekat bagian bawah.
- Error tampil dekat composer, dengan tombol coba lagi untuk mengulang pesan terakhir.
- Mode terang/gelap mengikuti sistem; reduced-motion dihormati; kontrol ikon punya label ARIA dan fokus terlihat.

## Ketahanan dan batasan

- Pesan kosong tidak dikirim. Input dibatasi 8.000 karakter.
- AbortController membatalkan stream aktif. Pesan assistant kosong dari request dibatalkan dihapus.
- Data chat hanya ada di browser melalui `localStorage`; tidak ada auth atau backend.

## Verifikasi

- `npm run build` harus lulus tanpa error TypeScript.
- Tambahkan test kecil untuk parser token/SSE bila setup test tidak menambah bloat; kalau tidak, build menjadi verifikasi runnable.

## Asumsi

- Agent menerima `POST` JSON `{ messages: [{ role, content }] }` dan merespons stream teks atau SSE `data: {"token":"..."}` lalu `[DONE]`.
- Tidak perlu backend, database, atau autentikasi karena `readme.md` meminta persistence localStorage.
