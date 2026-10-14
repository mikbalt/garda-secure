# Panduan perawatan GARDA

Untuk pemilik proyek. Isinya: apa yang perlu dicek rutin, cara menangani Pull
Request (PR) otomatis dari Dependabot, dan cara melanjutkan pekerjaan bersama
Claude atau developer lain. Detail teknis server ada di [runbook](runbook.md).

## Peta repo dalam 1 menit

| Mau tahu | Buka |
|---|---|
| Apa yang sudah selesai, apa yang tersisa | [PROGRESS.md](../PROGRESS.md) — bagian *Backlog* = daftar pekerjaan berikutnya |
| Kenapa sesuatu dibuat begitu | [DECISIONS.md](../DECISIONS.md) — setiap keputusan bernomor D-xxx |
| Aturan kerja untuk agent/developer | [CLAUDE.md](../CLAUDE.md) |
| Fitur dan kebutuhan produk | [PRD](../SecurePatrol%20PRD-v3.md) |
| Menjalankan dan merawat server | [runbook.md](runbook.md) |
| Panduan sales (PDF/HTML) | [sales-guide/](sales-guide/) |

Branch utama: **`init`**. Semua pekerjaan berakhir di sana.

## Rutin bulanan (± 15 menit)

1. **GitHub → tab *Actions*.** Pastikan workflow terakhir di `init` hijau (✓).
   Kalau merah (✗), minta Claude: *"cek kenapa CI merah di init dan perbaiki"*.
2. **GitHub → tab *Pull requests*.** Dependabot membuka paling banyak satu PR per
   bagian (backend, web, mobile, actions, docker) setiap awal bulan, isinya hanya
   update kecil (minor/patch). Lihat aturan di bawah.
3. **Server** (kalau sudah produksi): cek e-mail alert dari Alertmanager, status
   backup di *Pengaturan → Cadangan data*, dan sisa disk (runbook §8).

## Menangani PR Dependabot

Dependabot adalah bot GitHub yang mengusulkan versi baru library. Aturannya:

| Kondisi PR | Tindakan |
|---|---|
| Judulnya "Bump the … group" (update kecil) **dan** semua centang hijau ✓ | Klik **Merge pull request** → **Confirm**. Selesai. |
| Ada centang merah ✗ | Jangan di-merge. Tulis komentar `@dependabot close`, atau minta Claude memperbaikinya. |
| Ragu / tidak paham isinya | Minta Claude: *"tinjau PR Dependabot yang terbuka, merge yang aman, tutup sisanya"*. |

Update **besar** (major) sengaja tidak diusulkan Dependabot karena sering merusak
build. Itu dikerjakan terencana (lihat daftar di bawah). Keycloak juga tidak
pernah lewat Dependabot: setiap update Keycloak memigrasi database login dan harus
mengikuti runbook §2.

## Upgrade besar yang sudah direncanakan

Kerjakan satu per satu, masing-masing di sesi tersendiri bersama Claude, dan
selalu sebelum tanggal *end of life*-nya:

| Komponen | Sekarang | Target | Catatan |
|---|---|---|---|
| Node (build web) | 22 LTS | 24 LTS | Node 22 habis dukungan April 2027 |
| React | 18 | 19 | Bersamaan dengan library UI terkait |
| Vite / Zod / Tailwind | 7 / 3 / 3 | 8 / 4 / 4 | Tailwind 4 juga menghapus temuan `npm audit` di tools build |
| Flutter (tablet) | 3.24 | versi stabil terbaru | Membuka update `path`, `intl`, Firebase |
| Redis | 7 | 8 | Uji worker notifikasi |
| Grafana | 12 | 13 | Dashboard dicek ulang |
| Go | 1.26 | rilis berikutnya saat 1.26 habis dukungan | Kebijakan D-031 |
| Keycloak | 26.8 | patch 26.x berikutnya | Hanya lewat runbook §2 (backup → deploy → `make keycloak-smoke`) |

## Melanjutkan pekerjaan dengan Claude

Mulai sesi baru di folder repo, lalu tulis misalnya:

- *"Baca CLAUDE.md, PROGRESS.md, dan DECISIONS.md, lalu lanjutkan backlog yang paling penting."*
- *"Kerjakan item backlog: …"* (salin dari PROGRESS.md)
- *"Upgrade React ke 19 sesuai docs/maintenance.md."*

Claude mengikuti protokol di CLAUDE.md: klaim tugas di PROGRESS.md, uji, catat
keputusan, lalu push ke `init`.

## Kalau ada yang macet di PC developer

| Gejala | Penyebab biasa | Solusi |
|---|---|---|
| `npm ci` gagal *EPERM … esbuild.exe* | Masih ada `vite`/`npm run dev`/preview yang berjalan dan mengunci file | Tutup terminal yang menjalankannya (atau Task Manager → `node.exe`), lalu `cd web && npm ci` |
| Web di browser tidak bisa sampai ke beranda | Sisa login lama di browser | Buka `https://<server>/?reset` (runbook §11) |
| Banyak folder di `.claude/worktrees/` | Sisa kerja agent paralel | Aman dihapus jika branch-nya sudah di-merge: `git worktree prune` |
