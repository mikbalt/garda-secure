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
Tanpa keduanya, alat hanya tahu rentang yang **dikatakan** oleh pembuat bundel, lalu alat berakhir dengan `RANGE NOT AUTHENTICATED` (kode 11), tidak pernah `VALID`.
Pakai tanggal akhir yang sudah lewat minimal sehari: bundel harus berakhir di tanda tangan yang dibuat setelah rentang Anda, jadi rentang yang diharapkan
berakhir hari ini atau lebih lambat tidak akan pernah lolos (tanda tangan tidak bisa dibuat setelah hari ini berakhir). Hanya bila Anda benar-benar tidak bisa menyebut rentang,
tambahkan `--no-range-check`: hasilnya ditulis `VALID (range not checked)`.

| Hasil | Kode keluar | Artinya |
|---|---|---|
| `VALID` | 0 | Entri di bundel tersambung satu sama lain, catatan yang dibawanya cocok dengan entrinya, dan tanda tangannya asli serta cocok dengan rantai. Baca "Yang TIDAK dibuktikan VALID" di bawah sebelum Anda mengandalkannya. |
| `BROKEN` (catatan atau rantai) | 1 | Sebuah catatan atau entri diubah, dihapus, atau dipindah urutannya. Keluaran menyebut yang **pertama**: nomor urut, jenis (`attendance`, `scan`, `incident`, `incident_amendment`), id catatan, dan alasannya. |
| *(salah pemakaian)* | 2 | Argumen salah, misalnya tanpa `--pin`. |
| masalah bundel | 3 | File bukan bundel GARDA, rusak, tidak cocok dengan hash file-nya sendiri, atau rentangnya tidak sesuai dengan entrinya. (Hash file hanya menangkap kerusakan dan suntingan sembarangan.) |
| `BROKEN` (tanda tangan) | 4 | Tanda tangan tidak sah, kunci penanda tangan bukan yang Anda pin (**fingerprint salah**), atau rantai tidak lagi cocok dengan yang ditandatangani signer (masa lalu ditulis ulang). |
| `UNSIGNED` | 5 | Rantai punya entri tetapi belum ada tanda tangan sama sekali. Tidak ada yang menjangkarkannya. |
| `STALE` | 6 | Ada entri setelah tanda tangan terbaru, dan tanda tangan itu lebih tua dari `--max-age` (bawaan 26 jam). Biasanya signer berhenti. Ini bukan bukti manipulasi; tanyakan ke vendor. |
| `UNSIGNED TAIL` | 7 | Entri terakhir di bundel datang setelah tanda tangan terbaru, jadi belum ada tanda tangan yang menjamin. Bundel yang dipotong pendek tampak persis seperti ini. Sejak D-075 ekspor meminta signer menandatangani kepala rantai terbaru lebih dulu, jadi bundel yang jujur berakhir di tanda tangan dan kode ini **jarang**: artinya signer tidak menjawab atau menolak (keluaran dan `manifest.json` menyebut alasannya: `signing`, `signing_reason`), atau ada entri yang dibuat beberapa detik terakhir. Ekspor lagi setelah signer sehat. **Tidak valid.** |
| `UNVERIFIED RECORDS` | 8 | Sebuah catatan di bundel dikatakan berubah lagi, dan entri yang mengubahnya tidak ada di bundel, jadi isinya tidak bisa diperiksa. Sejak D-075 ekspor diteruskan sampai tanda tangan pertama (atau ujung rantai) yang mencakup setiap entri perubahan, jadi untuk ekspor yang jujur ini tidak lagi terjadi; kode ini tinggal untuk bundel yang dipotong, atau yang label "superseded"-nya tidak dibuktikan apa pun di bundel. `--allow-unverified-records` mengubah jawabannya menjadi kode 12 (`UNVERIFIED RECORDS ACCEPTED`), yang **tetap tidak valid**: keluaran menyebut berapa entri, dan berapa persen, yang tidak diperiksa. |
| `NO ENTRIES` | 9 | Bundel tidak punya entri. Ini tidak membuktikan apa pun tentang rentangnya: tanda tangan tidak bisa menunjukkan bahwa sebuah lokasi tidak punya entri. **Tidak valid.** |
| `RANGE MISMATCH` | 10 | Bundel tidak mencakup rentang yang Anda isi di `--expect-from` / `--expect-to`: dibuat untuk tanggal lain, mulai dari tanda tangan yang dibuat setelah rentang Anda dimulai, atau tanda tangan terbarunya lebih tua dari akhir rentang Anda. **Tidak valid.** |
| `RANGE NOT AUTHENTICATED` | 11 | Rantai dan tanda tangan benar, tetapi Anda tidak mengisi `--expect-from` / `--expect-to`, jadi rentangnya hanya kata pembuat bundel (bundel yang dipotong di tanda tangan lama, dimulai terlambat, atau bundel lama yang asli tampak sama). Isi rentangnya, atau terima dengan sadar lewat `--no-range-check` (kode 0, ditulis `VALID (range not checked)`). **Tidak valid.** |
| `UNVERIFIED RECORDS ACCEPTED` | 12 | Seperti 8, tetapi Anda memakai `--allow-unverified-records`. Sengaja bukan kode 0: opsi yang ditambahkan karena kebiasaan tidak boleh mengubahnya jadi lolos. |

Selain hasil, alat menampilkan kapan tanda tangan terbaru dibuat dan kapan bundel diekspor (menurut jam pembuat bundel), dan memberi peringatan bila bundel lebih tua dari `--max-age`: bundel lama tetap "valid" untuk isinya, tetapi tidak berkata apa-apa tentang kejadian sesudahnya. Opsi lain: `--json` (hasil untuk mesin), `--max-age 48h` (atau `0` untuk mematikan pemeriksaan ini), `--trust-first-use`
(menerima kunci pertama di bundel bila Anda tidak punya fingerprint; lebih lemah karena bundel menjamin dirinya sendiri).

**Catatan hari ini.** Ekspor ditandatangani saat dibuat, jadi menjamin entri sampai saat itu. Rentang yang belum berakhir
(`--expect-to` hari ini) tidak bisa diautentikasi, karena tidak ada tanda tangan yang menjamin entri sesudahnya: alat menjawab `RANGE MISMATCH` (kode 10).
Periksa sehari setelah harinya lewat (ekspor lagi setelah tengah malam: kepala rantai barunya ditandatangani setelah hari itu berakhir), atau pakai `--no-range-check` dengan sadar.

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
* **Catatan yang berubah lagi di luar bundel.** Bundel diteruskan sampai entri yang mengubah catatannya, jadi ini hanya tersisa untuk bundel yang dipotong; isinya tidak diperiksa (kode 8 kecuali Anda memilih menerimanya).
* **Bahwa bundel masih terbaru.** Bundel lama yang asli tetap valid selamanya. Lihat "Exported" dan "newest signed head".
* **Bahwa tidak ada pihak dengan akses penuh ke server dan ke mesin signer yang bertindak.** Siapa pun yang menguasai host sebagai root *dan* volume kunci signer
  bisa menandatangani riwayat yang ditulis ulang. Yang membatasinya: mem-pin fingerprint, menyimpan bundel lama milik Anda sendiri, dan serah terima kunci di hadapan klien.
* **Bahwa lokasi tanpa entri memang tidak punya entri.** Bundel kosong adalah kode 9, bukan valid.
* **Bahwa tidak ada yang melihat datanya.** Setiap ekspor (bundel atau paket bukti) menulis satu baris ke log ekspor (siapa, lokasi, rentang, waktu, ukuran, cara penandatanganan), tetapi log itu ada di database yang sama dengan catatannya, dan tidak berkata apa-apa tentang apa yang dilakukan dengan berkasnya; bundel berisi data pribadi (termasuk data Satpam).
* **Bahwa hash file di manifest melindungi bundel.** Hash itu hanya menunjukkan kerusakan atau suntingan sembarangan. Yang melindungi adalah rantai dan tanda tangan.

## Pemeriksaan online (tanpa alat)

`GET /api/v1/sites/<lokasi>/chain/verify?from=2026-11-01&to=2026-11-30&pin=<fingerprint>` menjawab `{"valid": true, "valid_pinned": true, "summary": {...}, ...}` atau catatan rusak pertama,
untuk admin, supervisor, dan client-readonly. Pemeriksaan mulai dari tanda tangan terakhir sebelum rentang Anda, jadi sebulan cepat. Jika rentang terlalu besar atau
terlalu lama, jawabannya `verify_too_large`: pakai bundel dan `garda-verify`. Pemeriksaan online mempercayai server yang Anda tanyai; pemeriksaan offline tidak,
karena itu gunakan yang offline untuk hal penting. Pemeriksaan ini tidak pernah meminta signer menandatangani.

**Baca `summary.state`** (dengan `summary.tone`: `ok`, `notice`, atau `broken`). Hanya `broken` yang berarti gagal:

| `summary.state` | `valid` | nada | Arti |
|---|---|---|---|
| `valid_pinned` | `true` | ok | Benar, dan kunci yang Anda pin menjaminnya. |
| `unverified_trust` | `null` | notice | Benar, tetapi tanpa `pin`: server menjamin dirinya sendiri. |
| `unsigned_tail` | `null` | notice | Sejauh ini tidak ada yang salah; entri terbaru belum tercakup tanda tangan (juga: belum ada tanda tangan, atau yang terbaru sudah lama). Periksa lagi nanti. |
| `unverified_records` | `null` | notice | Benar; beberapa catatan berubah lagi setelah rentang dan isinya tidak bisa diperiksa. |
| `no_entries` | `null` | notice | Tidak ada entri di rentang itu. |
| `broken` | `false` | broken | Ada entri, catatan, atau tanda tangan yang tidak cocok. `first_break` menyebut yang pertama. |

`status` tetap status mentah verifier. Antarmuka menampilkan `summary.state` dan tidak pernah mewarnai selain `broken` sebagai kegagalan.
Satu pengguna hanya boleh menjalankan satu pemeriksaan sekaligus dan beberapa per menit. Pemeriksaan online memakai tanggal yang diminta apa adanya, jadi tidak menjalankan pemeriksaan rentang yang diharapkan.

## Log ekspor

Setiap bundel dan paket bukti (dibuat atas permintaan, dibaca dari salinan tersimpan, atau disimpan penjadwal) menambah satu baris ke `export_audit`
(migrasi 015, hanya tambah seperti log audit): siapa, lokasi, rentang, kapan, ukuran dalam byte, hasil (`ok`, `too_large`, `failed`),
`signing` (cara mendapat kepala rantai baru: `signed`, `reused`, `unreachable`, `refused`, `not_configured`, atau `stored`) dan keterangan singkat. Tidak ada isi catatan.
`GET /api/v1/sites/<lokasi>/chain/exports` menampilkan barisnya (admin dan supervisor dari realm lokasi itu; lokasi realm lain 404). Bundel tidak dikirim
bila barisnya gagal ditulis.

## Format bundel (`garda-chain-bundle/v1`)

Zip berisi `manifest.json` (format, `site_id`, rentang, `anchor_seq`, `first_seq`, `last_seq`, kepala rantai saat ekspor, `signing` dan `signing_reason` (hasil permintaan tanda tangan baru ke signer; hanya penjelasan, putusan tetap dari tanda tangan), sha256 tiga file lain),
`entries.jsonl` (satu entri per baris: `seq`, `kind`, `record_id`, `record_hash`, `prev_hash`, `entry_hash`, `hash_version`, `origin`, `created_at`, `record_state`, dan teks `record`),
`heads.json` (tanda tangan kepala rantai, format sama dengan `signer export`) dan `keyring.json` (kunci **publik** signer). Tidak ada kunci privat di bundel.
Sebelum bundel dibuat, ekspor meminta signer (di jaringan internal server, `SIGNER_URL`) menandatangani kepala rantai saat itu; aturan penolakan signer tidak dilewati.
Bila signer tidak menjawab atau menolak, bundel tetap dibuat, berakhir di ekor tanpa tanda tangan, dan menyebut alasannya. Kepala yang ditandatangani 10 detik terakhir dipakai ulang untuk ekspor berikutnya dari lokasi yang sama.
Bila sebuah catatan di rentang diubah lagi oleh entri setelah kepala itu, bundel diteruskan sampai tanda tangan (atau ujung rantai) yang mencakup entri itu.
Rumus perhitungan ulang ada di [versi Inggris](chain-verification.md#bundle-format-garda-chain-bundlev1).

## Paket bukti bulanan (PDF)

`GET /api/v1/sites/<lokasi>/proof-pack?month=2026-11&lang=id` (admin, supervisor; Laporan → Paket bukti di aplikasi web)
mengembalikan PDF berisi ringkasan bulan itu, hash rantai, tanda tangan terbaru, sidik jari kunci, dan di halaman terakhir
langkah-langkah di atas. Penjadwal juga menyimpan paket bulan sebelumnya mulai hari pertama bulan. Paket ini memakai
pemeriksaan yang sama dengan `garda-verify` dan menyatakannya terus terang: **UNSIGNED TAIL** bila belum ada tanda tangan
setelah akhir bulan yang mencakup catatan terakhir, **PEMERIKSAAN RANTAI GAGAL** bila ada yang tidak cocok, **TIDAK ADA
AKTIVITAS** untuk bulan tanpa catatan. Paket yang "lolos" adalah server yang memeriksa dirinya sendiri: jalankan `garda-verify`
pada berkas ekspor dengan sidik jari yang Anda pegang. Lihat D-074.
