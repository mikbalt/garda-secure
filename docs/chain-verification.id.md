# Memeriksa bahwa catatan Anda tidak diubah

*English: [chain-verification.md](chain-verification.md)*

GARDA menyimpan setiap catatan absensi, scan patroli, dan insiden sebuah lokasi dalam satu rantai. Setiap entri
memuat sidik jari (hash) dari catatannya dan dari entri sebelumnya, jadi mengubah atau menghapus apa pun akan
memutus rantai sejak titik itu. Sebuah **signer** terpisah juga menandatangani entri terbaru setiap lokasi, dengan kunci
yang tidak bisa dijangkau aplikasi GARDA. Semua ini bisa Anda periksa sendiri dengan alat `garda-verify`, tanpa harus
percaya begitu saja pada vendor.

## Yang Anda perlukan

1. **Sidik jari (fingerprint) kunci signer.** 64 karakter. Anda menerimanya saat sistem dipasang (di atas kertas atau langsung).
   Simpan di tempat yang tidak bisa diubah vendor. Hanya ini yang harus Anda pegang sendiri: dengan ini tanda tangan palsu tidak berguna.
2. **Bundel ekspor** untuk lokasi dan tanggal yang ingin diperiksa (file `.zip`). Minta ke administrator GARDA Anda; admin atau supervisor
   juga bisa mengunduhnya (`GET /api/v1/sites/<lokasi>/chain/export?from=2026-11-01&to=2026-11-30`).
   Bundel berisi teks catatan, jadi perlakukan seperti catatannya sendiri.
3. **`garda-verify`**, satu file kecil untuk Linux, macOS, atau Windows. Tanpa instalasi, tanpa internet, tanpa akun. Alat ini hanya membaca bundel.

## Menjalankan

```
garda-verify --pin <fingerprint> garda-chain-1234abcd-2026-11-01_2026-11-30.zip
```

| Hasil | Kode keluar | Artinya |
|---|---|---|
| `VALID` | 0 | Setiap entri tersambung ke entri sebelumnya, setiap catatan di bundel cocok dengan entrinya, dan setiap tanda tangan asli serta cocok dengan rantai. |
| `BROKEN` (catatan atau rantai) | 1 | Sebuah catatan atau entri diubah, dihapus, atau dipindah urutannya. Keluaran menyebut yang **pertama**: nomor urut, jenis (`attendance`, `scan`, `incident`, `incident_amendment`), id catatan, dan alasannya. |
| *(salah pemakaian)* | 2 | Argumen salah, misalnya tanpa `--pin`. |
| masalah bundel | 3 | File bukan bundel GARDA, rusak, atau diubah setelah diekspor. |
| `BROKEN` (tanda tangan) | 4 | Tanda tangan tidak sah, kunci penanda tangan bukan yang Anda pin (**fingerprint salah**), atau rantai tidak lagi cocok dengan yang ditandatangani signer (masa lalu ditulis ulang). |
| `UNSIGNED` | 5 | Rantai punya entri tetapi belum ada tanda tangan sama sekali. Tidak ada yang menjangkarkannya. |
| `STALE` | 6 | Ada entri setelah tanda tangan terbaru, dan tanda tangan itu lebih tua dari `--max-age` (bawaan 26 jam). Biasanya signer berhenti. Ini bukan bukti manipulasi; tanyakan ke vendor. |

Opsi lain: `--json` (hasil untuk mesin), `--max-age 48h` (atau `0` untuk mematikan pemeriksaan ini), `--trust-first-use`
(menerima kunci pertama di bundel bila Anda tidak punya fingerprint; lebih lemah karena bundel menjamin dirinya sendiri).

Catat **sha256 bundel** yang ditampilkan alat. Jika nanti Anda menerima rentang yang sama, hash yang sama berarti bundel yang sama.

## Arti hasil VALID

* Artinya: pada rentang ini tidak ada yang diubah, dihapus, atau dipindah setelah entri ditandatangani, sejauh yang bisa ditunjukkan kunci signer dan rantai.
  Catatan yang diubah di database lalu disambung ulang agar tampak normal tetap ketahuan, karena tanda tangan masa lalu tidak lagi cocok.
* Catatan yang diubah **lewat aplikasi** (koreksi supervisor, absen pulang Satpam) bukan manipulasi: itu menambah entri baru dan entri lama tetap ada.
* Keadaan lama sebuah catatan tidak disimpan. Untuk catatan yang berubah lagi setelah rentang tanggal Anda, alat hanya memeriksa entrinya sebagai mata rantai.
  Keluaran menyebut jumlahnya (`records_not_checked`).
* Entri setelah tanda tangan terbaru (sekitar satu jam) belum ditandatangani; diperiksa sebagai mata rantai, dan `STALE` muncul bila signer diam terlalu lama.
* Jika kunci signer hilang atau bocor, vendor memulai epoch baru dan Anda harus mem-pin fingerprint baru (diberikan lagi secara langsung). Sampai itu, alat melapor kode 4.

## Pemeriksaan online (tanpa alat)

`GET /api/v1/sites/<lokasi>/chain/verify?from=2026-11-01&to=2026-11-30&pin=<fingerprint>` menjawab `{"valid": true, ...}` atau catatan rusak pertama,
untuk admin, supervisor, dan client-readonly. Pemeriksaan mulai dari tanda tangan terakhir sebelum rentang Anda, jadi sebulan cepat. Jika rentang terlalu besar atau
terlalu lama, jawabannya `verify_too_large`: pakai bundel dan `garda-verify`. Pemeriksaan online mempercayai server yang Anda tanyai; pemeriksaan offline tidak,
karena itu gunakan yang offline untuk hal penting.

## Format bundel (`garda-chain-bundle/v1`)

Zip berisi `manifest.json` (format, `site_id`, rentang, `anchor_seq`, `first_seq`, `last_seq`, kepala rantai saat ekspor, sha256 tiga file lain),
`entries.jsonl` (satu entri per baris: `seq`, `kind`, `record_id`, `record_hash`, `prev_hash`, `entry_hash`, `hash_version`, `origin`, `created_at`, `record_state`, dan teks `record`),
`heads.json` (tanda tangan kepala rantai, format sama dengan `signer export`) dan `keyring.json` (kunci **publik** signer). Tidak ada kunci privat di bundel.
Rumus perhitungan ulang ada di [versi Inggris](chain-verification.md#bundle-format-garda-chain-bundlev1).
