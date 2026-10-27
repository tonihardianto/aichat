# Desain: Chat AI produksi dengan Go dan MySQL

**Status:** Diimplementasikan

## Intent

Ubah UI chat React saat ini dari aplikasi browser-lokal menjadi layanan multi-pengguna yang menyimpan data secara aman di server. Go menjadi pemilik API, autentikasi, percakapan, lampiran, konfigurasi provider AI, serta proxy streaming. MySQL menjadi sumber data transaksi. Frontend React/Vite tetap dipakai dan hanya mengakses API same-origin.

Motivasi utama: riwayat tidak lagi hilang karena kuota `localStorage`, URL/provider AI tidak diekspos ke browser, lampiran tidak diserialisasi berulang per token, dan produk punya fondasi untuk deployment, observability, serta pengujian.

## Sasaran

1. Pengguna dapat daftar, masuk, keluar, lalu melihat hanya percakapan miliknya.
2. Pengguna dapat membuat, mengganti nama, menghapus, dan membuka percakapan.
3. Pengguna dapat mengunggah gambar JPEG, PNG, atau WebP, lalu mengirim pesan dengan satu atau lebih lampiran.
4. Backend menyimpan pesan user dan melakukan stream respons model melalui SSE.
5. Riwayat, lampiran, dan pesan parsial tersimpan di server; reload atau perangkat lain tidak menghilangkan data.
6. Provider Ollama/OpenAI-compatible hanya dihubungi server. Endpoint dan credential provider tidak pernah masuk bundle frontend.
7. Sistem dapat dijalankan lokal lewat Docker Compose dan dideploy sebagai satu service API + MySQL + object storage S3-compatible.

## Di luar cakupan rilis pertama

- OAuth/SSO, organisasi, role admin UI, billing, sharing percakapan, pencarian semantik/RAG, tool calling, multi-provider UI, dan aplikasi mobile.
- Sinkronisasi real-time lintas tab/perangkat selain data terbaru saat reload.
- Upload dokumen selain gambar.
- Menyimpan chain-of-thought atau metadata internal provider.

## Keputusan desain

### Topologi

```text
Browser (React/Vite)
  └── HTTPS same-origin /api/*
        └── Go service
              ├── MySQL 8.0: users, sessions, conversations, messages, attachments
              ├── S3-compatible object storage: blob gambar
              └── Ollama/OpenAI-compatible API: server-side streaming
```

- Produksi: frontend statis di-serve reverse proxy/CDN; `/api` diproxy ke Go.
- Pengembangan: Vite proxy `/api` ke Go. Tidak ada `VITE_OLLAMA_BASE_URL` atau `VITE_AGENT_URL`.
- Satu API monolitik. Tidak membuat microservice, queue, Redis, atau event bus sebelum ada kebutuhan terukur.
- Go memakai `net/http` dan `database/sql`; dependency eksternal hanya driver MySQL, client S3, dan bcrypt. Router/framework tidak diperlukan pada tahap ini.

### Identitas dan keamanan sesi

- Rilis pertama memakai email + password.
- Password di-hash dengan bcrypt; plaintext tidak pernah disimpan atau dicatat.
- Login menghasilkan token acak 32 byte. Database hanya menyimpan hash token, user ID, expiry, waktu revoke, dan metadata audit minimum.
- Browser menerima cookie `HttpOnly`, `Secure` pada HTTPS, `SameSite=Lax`, `Path=/api`. Cookie tidak dapat dibaca JavaScript.
- Semua endpoint mutasi memeriksa `Origin` terhadap origin aplikasi, selain memerlukan sesi valid. Ini menjadi proteksi CSRF untuk cookie session.
- Password minimal 12 karakter. Pesan error login tidak membedakan email tidak ada dan password salah.
- Rilis pertama tidak mencakup verifikasi email atau reset password. Copy UI menyatakan akun tidak dapat dipulihkan; fitur itu ditambah hanya bersama layanan email terverifikasi.
- Rate limit in-memory per IP untuk login/registrasi dan per user untuk chat/upload. Batas konfigurabel via environment. Untuk satu instance cukup; deployment multi-instance kelak perlu rate limiter bersama.

### Provider AI

- Konfigurasi provider disimpan di environment server: `LLM_BASE_URL`, `LLM_API_KEY` bila dibutuhkan, dan daftar model yang diizinkan (`LLM_ALLOWED_MODELS`).
- Implementasi pertama mendukung format OpenAI-compatible `GET /models` dan `POST /chat/completions` dengan streaming SSE, sehingga cocok untuk Ollama `/v1`.
- `GET /api/v1/models` hanya menampilkan irisan model provider dengan allowlist. Klien tidak mengirim base URL atau credential.
- API memvalidasi model terhadap allowlist sebelum menghubungi provider.
- Backend meneruskan hanya role `user` dan `assistant`, konten pesan tersimpan, dan referensi attachment milik user. Pesan kosong ditolak.

### Penyimpanan data

MySQL adalah sumber kebenaran metadata. Waktu disimpan UTC (`DATETIME(6)`). UUID v7 atau ULID dipakai sebagai ID publik; rilis pertama dapat memakai UUID v4 bila library standar dipilih konsisten.

| Tabel | Kolom inti | Catatan |
|---|---|---|
| `users` | `id`, `email`, `password_hash`, `created_at`, `updated_at` | `email` unique dengan collation case-insensitive yang dipilih eksplisit. |
| `sessions` | `id`, `user_id`, `token_hash`, `expires_at`, `revoked_at`, `created_at`, `last_seen_at` | Index `token_hash` unique dan `(user_id, expires_at)`. |
| `conversations` | `id`, `user_id`, `title`, `created_at`, `updated_at`, `deleted_at` | Index `(user_id, updated_at DESC)`; soft delete untuk recovery operasional. |
| `messages` | `id`, `conversation_id`, `role`, `content`, `status`, `model`, `input_tokens`, `output_tokens`, `tokens_per_second`, `created_at`, `completed_at` | `status`: `complete`, `streaming`, `cancelled`, `failed`; index `(conversation_id, created_at, id)`. |
| `attachments` | `id`, `user_id`, `object_key`, `original_name`, `content_type`, `byte_size`, `sha256`, `created_at`, `deleted_at` | Blob tidak masuk MySQL. Index `(user_id, created_at DESC)`. |
| `message_attachments` | `message_id`, `attachment_id`, `position` | Primary key komposit; ownership diverifikasi di transaksi. |

- Blob gambar masuk bucket privat S3-compatible. Object key dihasilkan server, tidak memakai nama file user.
- Download/preview memakai endpoint terautentikasi yang memeriksa ownership lalu mengalirkan objek; tidak memakai URL bucket publik.
- Rilis pertama membatasi maksimal 10 MiB per gambar dan maksimal 4 lampiran per pesan. Server mendeteksi tipe dari byte file, bukan hanya header atau ekstensi.
- Percakapan di-soft-delete selama 30 hari. Attachment yang hanya direferensikan percakapan terhapus menjadi soft-deleted bersamanya dan objeknya dihapus setelah 30 hari. Attachment yatim yang tidak pernah dikirim dihapus setelah 24 jam.
- Pekerja pembersih periodik menjalankan aturan retensi itu. Ia adalah pekerjaan terjadwal di proses Go, bukan cron eksternal pada awalnya.

### API HTTP

Seluruh respons JSON memakai bentuk `{ "data": ... }` atau `{ "error": { "code", "message" } }`. Server memberi `X-Request-ID`; log memakai ID itu dan tidak pernah memuat isi chat, cookie, token, atau blob.

| Method | Path | Tujuan |
|---|---|---|
| `POST` | `/api/v1/auth/register` | Buat akun dan sesi. |
| `POST` | `/api/v1/auth/login` | Buat sesi. |
| `POST` | `/api/v1/auth/logout` | Revoke sesi aktif dan clear cookie. |
| `GET` | `/api/v1/auth/me` | Identitas sesi sekarang. |
| `GET` | `/api/v1/models` | Model yang boleh dipilih. |
| `POST` | `/api/v1/attachments` | Upload multipart, buat attachment private. |
| `GET` | `/api/v1/attachments/{id}/content` | Stream preview attachment setelah cek owner. |
| `GET` | `/api/v1/conversations` | Daftar paginated milik user. |
| `POST` | `/api/v1/conversations` | Buat percakapan. |
| `GET` | `/api/v1/conversations/{id}` | Detail percakapan dengan pesan dan attachment metadata. |
| `PATCH` | `/api/v1/conversations/{id}` | Ubah judul. |
| `DELETE` | `/api/v1/conversations/{id}` | Soft delete percakapan. |
| `POST` | `/api/v1/conversations/{id}/messages:stream` | Simpan pesan user lalu stream jawaban asisten dalam satu koneksi SSE. |
| `POST` | `/api/v1/conversations/{id}/messages/{messageId}:regenerate` | Buat jawaban baru dari riwayat sampai pesan user sebelumnya. |
| `POST` | `/api/v1/conversations/{id}/messages/{messageId}:cancel` | Batalkan job aktif jika request stream masih hidup pada instance yang sama. |

Payload pesan stream berisi `content`, `model`, dan `attachmentIds`; `content` maksimum 8.000 karakter Unicode, dan gabungan riwayat yang diteruskan ke provider dibatasi 32.000 karakter sebagai pagar awal sampai tokenisasi per model diperlukan. Server meminta maksimal 4.096 output token bila provider mendukung parameter itu. Server mengembalikan 409 bila percakapan sudah memiliki generation `streaming`; UI menonaktifkan kirim hingga selesai.

Request stream menerima header `Idempotency-Key` UUID. Backend menyimpan key per percakapan dan user selama 24 jam; retry dengan key sama meneruskan/mengembalikan generation yang sama, bukan membuat pesan atau inferensi duplikat.

Endpoint `:regenerate` menerima ID pesan asisten `complete`, menyimpan respons lama apa adanya, lalu membuat pesan asisten baru setelah pesan user yang sama. Frontend menampilkan respons terbaru; pemilihan varian respons bukan target rilis pertama.

### Kontrak SSE

Server mengirim `Content-Type: text/event-stream`, `Cache-Control: no-cache`, dan flush setiap event.

```text
event: message.created
data: {"message":{"id":"...","role":"assistant","content":"","status":"streaming"}}

event: message.delta
data: {"messageId":"...","delta":"Halo"}

event: message.completed
data: {"message":{"id":"...","status":"complete","outputTokens":12,"tokensPerSecond":25.4}}

```

Jika provider atau penyimpanan gagal, server menulis `event: message.failed` dengan pesan aman untuk user, menandai pesan `failed`, lalu menutup stream. Jika browser membatalkan, context dibatalkan ke provider, konten yang sudah terkumpul disimpan sebagai `cancelled`. Pesan user tidak dihapus.

Saat API startup, recovery menandai semua pesan `streaming` yang dibuat lebih dari lima menit lalu sebagai `failed` dengan kode `generation_interrupted`; konten parsial tetap dapat dibaca. Generation tidak di-resume otomatis setelah process restart karena upstream stream tidak dapat dipulihkan dengan aman.

Konten asisten dibuffer di memori dan dipersistensi berkala maksimal setiap 500 ms atau setiap 1 KiB, lalu sekali lagi saat selesai. Ini menghindari write MySQL per token sambil menjaga respons parsial saat proses berhenti.

### Alur kirim pesan

1. Frontend mengunggah gambar lebih dulu lewat `/attachments`; hanya ID attachment masuk state chat.
2. Frontend `POST` ke endpoint `messages:stream` dan mulai membaca SSE.
3. Backend memverifikasi sesi, ownership conversation/attachment, batas rate, batas pesan, dan allowlist model.
4. Dalam transaksi singkat, backend menulis pesan user, relasi attachment, dan placeholder assistant `streaming`.
5. Backend meminta stream provider memakai riwayat lengkap yang telah dibaca dari database, lalu menerjemahkan delta ke kontrak SSE milik aplikasi.
6. Backend memperbarui konten assistant secara periodik, mengirim delta ke klien, lalu menandai terminal status `complete`, `failed`, atau `cancelled`.
7. Frontend menggabungkan delta secara lokal untuk rendering. Setelah terminal event atau reload, frontend membaca kembali data kanonis dari API.

### Frontend

- Hapus `useConversations` berbasis `localStorage`, `useModels` yang memanggil provider langsung, dan `lib/agent.ts` yang berisi credential/transport provider.
- Ganti dengan klien API kecil (`fetch` same-origin) dan hooks query/mutasi berbasis React state. Tidak menambah global state manager.
- Riwayat dimuat dari API saat aplikasi dibuka dan percakapan detail dimuat ketika dipilih.
- Composer mengunggah file sebagai `FormData`; state hanya menyimpan metadata/ID dan URL preview object lokal selama upload. Tidak menyimpan data URL riwayat.
- SSE dibaca dari `fetch` karena request awal adalah POST. `AbortController` membatalkan stream dan memanggil endpoint cancel bila perlu.
- UI auth rilis pertama mencakup login, registrasi, logout, dan state loading/error. Tidak ada mode demo di build produksi.
- Tema boleh tetap di `localStorage`; ia bukan data bisnis dan ukurannya kecil.

### Paket Go

```text
backend/
  cmd/api/main.go                 # konfigurasi, dependency wiring, shutdown
  internal/config/                # environment tervalidasi
  internal/httpapi/               # route, handler, middleware, JSON/SSE
  internal/auth/                  # password, session, middleware
  internal/chat/                  # use cases dan aturan ownership
  internal/provider/              # interface chat provider + OpenAI-compatible adapter
  internal/storage/               # interface object store + S3 adapter
  internal/repository/            # query MySQL dan transaksi
  internal/database/migrations/   # SQL up/down berurutan
  internal/observability/         # request ID, structured log, health/readiness
  Dockerfile
```

Domain dan handler tidak mengimpor driver MySQL atau client S3 secara langsung. Provider dan object store berada di belakang interface kecil yang dibutuhkan use case agar dapat diuji dengan fake in-memory.

### Operasional

- `docker-compose.yml` menjalankan MySQL, MinIO untuk development, API Go, dan frontend/Vite atau static proxy sesuai mode.
- Health: `/healthz` hanya proses hidup; `/readyz` memeriksa koneksi MySQL dan bucket storage. Endpoint ini tidak butuh sesi.
- Config environment divalidasi saat start. API gagal start jika session secret, DSN, bucket, atau provider URL tidak valid.
- Migrations dijalankan sebagai langkah deployment eksplisit dan aman idempotensi; tidak otomatis dari proses API pada produksi.
- Log structured JSON ke stdout dengan request ID, method, path template, status, durasi, dan error code. Tidak log body pesan, attachment name, token, password, atau API key.
- Backup: backup MySQL harian dan lifecycle/versioning bucket dikonfigurasi infrastruktur. Restore diuji sebelum produksi.

## Kualitas dan pengujian

- Go: `go test ./...`, race detector pada CI untuk package concurrent, test handler dengan `httptest`, test repository pada MySQL ephemeral/container, dan fake provider yang mengirim SSE terfragmentasi/error/abort.
- Frontend: Vitest + React Testing Library untuk auth, list percakapan, Composer upload/error, dan state SSE. Unit test parser event dan API client.
- End-to-end: Playwright pada stack Compose untuk register/login, upload, stream, cancel, reload riwayat, dan isolasi user A/B.
- CI: format (`gofmt`, `go vet`), test frontend/backend, build image, dependency vulnerability scan, migration smoke test. Build gagal bila test gagal.

## Kriteria sukses rilis pertama

- Pengguna terautentikasi hanya dapat membaca/mengubah resource miliknya; percobaan ID milik user lain menghasilkan 404.
- Riwayat teks dan attachment tetap ada setelah reload dan login dari browser lain.
- Lampiran 10 MiB tidak pernah masuk `localStorage` atau MySQL blob.
- Respons 1.000 token tidak menyebabkan write database per token atau serialisasi riwayat di browser per token.
- Batal stream menghentikan upstream dan menghasilkan pesan `cancelled` dengan konten parsial bila ada.
- URL dan API key provider tidak ditemukan pada bundle frontend atau network request browser.
- Semua test unit/integrasi/E2E yang didefinisikan di atas berjalan di CI.

## Call yang dibuat untuk menghindari scope kabur

- Menganggap produk perlu akun lokal email/password sejak awal; OAuth dapat ditambah belakangan.
- Menetapkan satu deployment API instance sebagai target awal. Cancel lintas instance bukan target sampai ada horizontal scaling.
- Memilih S3-compatible storage (MinIO lokal) dibanding filesystem agar deployment produksi tidak perlu migrasi blob kedua kali.
- Menetapkan adaptor OpenAI-compatible untuk Ollama, bukan membuat protokol Ollama khusus.
- Menetapkan satu provider dan allowlist model dari env, tanpa panel admin/provider switcher.
