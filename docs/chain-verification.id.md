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
garda-verify --pin <fingerprint> --expect-from 2026-11-01 --expect-to 2026-11-30 garda-chain-1234abcd-2026-11-01_2026-11-30.zip
```

Selalu isi `--expect-from` dan `--expect-to` (hari pertama dan hari terakhir yang Anda minta, menurut zona waktu lokasi).
Tanpa keduanya, alat hanya tahu rentang yang **dikatakan** oleh pembuat bundel, dan menulis `NOT AUTHENTICATED` di sebelahnya.
Pakai tanggal akhir yang sudah lewat minimal sehari: bundel harus berakhir di tanda tangan yang dibuat setelah rentang Anda.

| Hasil | Kode keluar | Artinya |
|---|---|---|
| `VALID` | 0 | Entri di bundel tersambung satu sama lain, catatan yang dibawanya cocok dengan entrinya, dan tanda tangannya asli serta cocok dengan rantai. Baca "Yang TIDAK dibuktikan VALID" di bawah sebelum Anda mengandalkannya. |
| `BROKEN` (catatan atau rantai) | 1 | Sebuah catatan atau entri diubah, dihapus, atau dipindah urutannya. Keluaran menyebut yang **pertama**: nomor urut, jenis (`attendance`, `scan`, `incident`, `incident_amendment`), id catatan, dan alasannya. |
| *(salah pemakaian)* | 2 | Argumen salah, misalnya tanpa `--pin`. |
| masalah bundel | 3 | File bukan bundel GARDA, rusak, tidak cocok dengan hash file-nya sendiri, atau rentangnya tidak sesuai dengan entrinya. (Hash file hanya menangkap kerusakan dan suntingan sembarangan.) |
| `BROKEN` (tanda tangan) | 4 | Tanda tangan tidak sah, kunci penanda tangan bukan yang Anda pin (**fingerprint salah**), atau rantai tidak lagi cocok dengan yang ditandatangani signer (masa lalu ditulis ulang). |
| `UNSIGNED` | 5 | Rantai punya entri tetapi belum ada tanda tangan sama sekali. Tidak ada yang menjangkarkannya. |
| `STALE` | 6 | Ada entri setelah tanda tangan terbaru, dan tanda tangan itu lebih tua dari `--max-age` (bawaan 26 jam). Biasanya signer berhenti. Ini bukan bukti manipulasi; tanyakan ke vendor. |
| `UNSIGNED TAIL` | 7 | Entri terakhir di bundel datang setelah tanda tangan terbaru, jadi belum ada tanda tangan yang menjamin. Bundel yang dipotong pendek tampak persis seperti ini. Ekspor lagi setelah tanda tangan berikutnya (dibuat sekitar tiap jam). **Tidak valid.** |
| `UNVERIFIED RECORDS` | 8 | Ada catatan yang dikatakan berubah lagi setelah bundel, dan entri barunya tidak ada di bundel, jadi isinya tidak bisa diperiksa. Ekspor rentang yang memuat perubahannya. Dengan `--allow-unverified-records` Anda menerimanya dengan sadar; hasilnya berbunyi `VALID WITH UNVERIFIED RECORDS`. **Tidak valid.** |
| `NO ENTRIES` | 9 | Bundel tidak punya entri. Ini tidak membuktikan apa pun tentang rentangnya: tanda tangan tidak bisa menunjukkan bahwa sebuah lokasi tidak punya entri. **Tidak valid.** |
| `RANGE MISMATCH` | 10 | Bundel tidak mencakup rentang yang Anda isi di `--expect-from` / `--expect-to`: dibuat untuk tanggal lain, mulai dari tanda tangan yang dibuat setelah rentang Anda dimulai, atau tanda tangan terbarunya lebih tua dari akhir rentang Anda. **Tidak valid.** |

Selain hasil, alat menampilkan kapan tanda tangan terbaru dibuat dan kapan bundel diekspor (menurut jam pembuat bundel), dan memberi peringatan bila bundel lebih tua dari `--max-age`: bundel lama tetap "valid" untuk isinya, tetapi tidak berkata apa-apa tentang kejadian sesudahnya. Opsi lain: `--json` (hasil untuk mesin), `--max-age 48h` (atau `0` untuk mematikan pemeriksaan ini), `--trust-first-use`
(menerima kunci pertama di bundel bila Anda tidak punya fingerprint; lebih lemah karena bundel menjamin dirinya sendiri).

Catat **sha256 bundel** yang ditampilkan alat. Jika nanti Anda menerima rentang yang sama, hash yang sama berarti bundel yang sama.

## Arti hasil VALID

* Artinya: di bundel ini tidak ada entri yang diubah, dihapus, atau dipindah setelah ditandatangani, sejauh yang bisa ditunjukkan kunci signer dan rantai.
  Catatan yang diubah di database lalu disambung ulang agar tampak normal tetap ketahuan, karena tanda tangan masa lalu tidak lagi cocok.
* Catatan yang diubah **lewat aplikasi** (koreksi supervisor, absen pulang Satpam) bukan manipulasi: itu menambah entri baru dan entri lama tetap ada.
* Keadaan lama sebuah catatan tidak disimpan. Untuk catatan yang berubah lagi di dalam bundel, entri lamanya hanya diperiksa sebagai mata rantai.
* Jika kunci signer hilang atau bocor, vendor memulai epoch baru dan Anda harus mem-pin fingerprint baru (diberikan lagi secara langsung). Sampai itu, alat melapor kode 4.

## Yang TIDAK dibuktikan VALID

`VALID` adalah pernyataan kuat tentang entri yang ada di bundel. Itu bukan pernyataan tentang semua yang terjadi. Khususnya, `VALID` **tidak** membuktikan:

* **Bahwa tidak ada yang hilang.** Bundel yang berhenti lebih awal, atau membuang catatan terakhir, tetap bisa `VALID` bila berhenti tepat di sebuah tanda tangan.
  Isi `--expect-to` (alat lalu butuh tanda tangan yang dibuat *setelah* rentang Anda) dan periksa apakah jumlah entri masuk akal untuk periodenya.
  Entri setelah tanda tangan terbaru dilaporkan sebagai `UNSIGNED TAIL`, bukan valid.
* **Bahwa tanggalnya sesuai permintaan Anda.** Rentang yang tertulis di bundel adalah klaim pembuatnya. Hanya `--expect-from` dan `--expect-to` yang memeriksanya terhadap waktu tanda tangan.
* **Catatan yang berubah lagi di luar bundel.** Isinya tidak diperiksa (kode 8 kecuali Anda memilih menerimanya).
* **Bahwa bundel masih terbaru.** Bundel lama yang asli tetap valid selamanya. Lihat "Exported" dan "newest signed head".
* **Bahwa tidak ada pihak dengan akses penuh ke server dan ke mesin signer yang bertindak.** Siapa pun yang menguasai host sebagai root *dan* volume kunci signer
  bisa menandatangani riwayat yang ditulis ulang. Yang membatasinya: mem-pin fingerprint, menyimpan bundel lama milik Anda sendiri, dan serah terima kunci di hadapan klien.
* **Bahwa lokasi tanpa entri memang tidak punya entri.** Bundel kosong adalah kode 9, bukan valid.
* **Bahwa tidak ada yang melihat datanya.** Ekspor belum dicatat dalam log; bundel berisi data pribadi (termasuk data Satpam).
* **Bahwa hash file di manifest melindungi bundel.** Hash itu hanya menunjukkan kerusakan atau suntingan sembarangan. Yang melindungi adalah rantai dan tanda tangan.

## Pemeriksaan online (tanpa alat)

`GET /api/v1/sites/<lokasi>/chain/verify?from=2026-11-01&to=2026-11-30&pin=<fingerprint>` menjawab `{"valid": true, "valid_pinned": true, ...}` atau catatan rusak pertama,
untuk admin, supervisor, dan client-readonly. Pemeriksaan mulai dari tanda tangan terakhir sebelum rentang Anda, jadi sebulan cepat. Jika rentang terlalu besar atau
terlalu lama, jawabannya `verify_too_large`: pakai bundel dan `garda-verify`. Pemeriksaan online mempercayai server yang Anda tanyai; pemeriksaan offline tidak,
karena itu gunakan yang offline untuk hal penting. Tanpa `pin`, jawabannya `"valid": null` dengan `"trust": "unverified_trust"` (server menjamin dirinya sendiri), tidak pernah `true`.
Satu pengguna hanya boleh menjalankan satu pemeriksaan sekaligus dan beberapa per menit.

## Format bundel (`garda-chain-bundle/v1`)

Zip berisi `manifest.json` (format, `site_id`, rentang, `anchor_seq`, `first_seq`, `last_seq`, kepala rantai saat ekspor, sha256 tiga file lain),
`entries.jsonl` (satu entri per baris: `seq`, `kind`, `record_id`, `record_hash`, `prev_hash`, `entry_hash`, `hash_version`, `origin`, `created_at`, `record_state`, dan teks `record`),
`heads.json` (tanda tangan kepala rantai, format sama dengan `signer export`) dan `keyring.json` (kunci **publik** signer). Tidak ada kunci privat di bundel.
Rumus perhitungan ulang ada di [versi Inggris](chain-verification.md#bundle-format-garda-chain-bundlev1).
