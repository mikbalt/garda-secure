# Catatan GARDA: apa yang ketahuan kalau diubah, dan apa yang tidak

*English: [explainer.md](explainer.md)* · Untuk klien atau pembicaraan penjualan. Tanda seperti **[G1]** menunjuk bukti di [claims.md](claims.md).

Catatan GARDA **ketahuan kalau diubah (tamper-evident), dan dijangkarkan di luar aplikasi**. Artinya: kalau sebuah catatan diubah atau
dihapus sesudahnya, pemeriksaan akan menunjukkannya, dan pemeriksaan itu tidak bergantung pada kata-kata kami. Ini **tidak** berarti catatan
mustahil diubah.

## Apa yang dicatat GARDA

Setiap absen masuk dan pulang, setiap scan titik cek saat patroli, setiap insiden, dan setiap koreksi insiden sesudahnya **[G1]**.
Kode QR di titik cek punya batas waktu berlaku. Scan dengan kode yang sudah lama atau salah, sinkronisasi terlambat, atau jam tablet yang
janggal **tetap disimpan dan diberi tanda** supaya supervisor memeriksanya; scan itu tidak dibuang. Menandai sebuah tanda sebagai "sudah
diperiksa" tidak mengubah catatannya **[F1]**.

## Bagaimana catatan dirantai dan ditandatangani

1. Setiap kali catatan seperti itu disimpan, GARDA menambah satu baris ke **rantai** lokasi itu, pada langkah yang sama. Baris itu berisi sidik
   jari (hash) catatan tersebut dan baris sebelumnya **[G1]**.
2. Kalau catatan atau baris diubah atau dihapus kemudian, bahkan langsung di basis data, rantai tidak lagi cocok mulai dari titik itu, dan
   pemeriksaan menyebut catatan pertama yang berubah **[G2]**.
3. Koreksi lewat aplikasi (perbaikan supervisor, absen pulang) bukan manipulasi. Koreksi menambah baris baru dan baris lama tetap ada,
   jadi riwayatnya terlihat **[G3]**.
4. Kira-kira setiap jam, dan di akhir hari setiap lokasi, **program terpisah bernama signer** menandatangani baris terbaru dengan kunci yang
   tidak bisa dipakai aplikasi. Signer berjalan sebagai proses sendiri, dengan file kunci sendiri dan akun basis data sendiri **[G4]**.
5. Signer menolak menandatangani riwayat yang lebih pendek atau berbeda. Signer juga menyimpan daftar sendiri tentang apa yang pernah ia
   tandatangani, di disknya sendiri, jadi menghapus salinan di basis data tidak membuatnya lupa **[G6]**.
6. Riwayat yang ditulis ulang dan disambung lagi supaya terlihat rapi tidak lagi cocok dengan tanda tangan sebelumnya, dan pemeriksaan gagal **[G5]**.

## Apa yang bisa diperiksa klien sendiri

Dengan program kecil `garda-verify`, sebuah file ekspor (`.zip`), dan **sidik jari kunci signer yang diterima saat pemasangan**, klien bisa
memeriksa di komputernya sendiri: tanpa akun, tanpa internet, tanpa sistem GARDA **[G7]**. Hasilnya `VALID`, atau catatan pertama yang tidak
lagi cocok. Sidik jari yang salah akan gagal **[G7]**. Alat ini tidak menulis `VALID` biasa untuk file kosong, untuk catatan sesudah tanda tangan
terakhir, untuk catatan yang perubahannya ada di luar file, atau untuk rentang tanggal yang tidak diminta dicocokkan **[G8]**.
PDF bukti bulanan mengulang pemeriksaan ini dan mencetak langkahnya, tetapi itu server yang memeriksa dirinya sendiri. Yang menentukan adalah
pemeriksaan klien sendiri **[P1]**.

## Yang TIDAK dijamin

| | |
|---|---|
| **Administrator server ditambah kunci signer.** | Orang yang punya akses administrator penuh ke server *dan* penyimpanan kunci signer bisa menandatangani riwayat yang ditulis ulang. Sidik jari milik klien sendiri, ekspor lama yang disimpan klien, dan pergantian kunci yang dilakukan di depan klien membatasi hal ini **[L1]**. |
| **Catatan sesudah tanda tangan terakhir.** | Catatan terbaru, sampai tanda tangan berikutnya (kira-kira satu jam), belum dijamin. Alat menampilkan `UNSIGNED TAIL` dan tidak menyebutnya valid; file yang terpotong juga tampak sama **[L2]**. |
| **Ekspor lama yang dipakai seolah baru.** | File lama yang asli tetap valid selamanya untuk isinya. Lihat tanggal ekspor dan tanggal tanda tangannya **[L3]**. |
| **Angka di PDF bukti bulanan.** | Total absensi, patroli, insiden, dan tanda diambil dari basis data saat PDF dibuat. Angka itu tidak ditandatangani, dan PDF menyatakannya **[L4]**. |
| **Foto kode QR yang dicetak.** | Stiker tidak bisa berubah. Salinannya bisa dipakai ulang sampai batas waktu kode habis (30 hari secara bawaan) ditambah sekitar 24 jam **[L5]**. |
| **Satpam yang offline dan berbohong soal jam.** | Tablet yang offline dengan jam yang diundur bisa memakai ulang foto itu sampai batas yang sama. Tanda peringatan serta pemeriksaan GPS dan foto memberi sinyal; scan tidak diblokir **[L6]**. |
| **Apakah isi yang dimasukkan itu benar.** | Rantai menunjukkan catatan tidak berubah sesudah disimpan, bukan bahwa catatan itu benar saat dimasukkan. Orang yang memakai aplikasinya sendiri masih bisa menambah baris baru yang salah; baris itu tampak sebagai baris baru dengan waktunya **[L7]**. |
| **Sidik jari yang Anda pegang.** | Hanya sidik jari yang diterima klien lewat jalur lain yang berlaku. Sidik jari yang tercetak di PDF adalah klaim server sendiri **[L8]**. |

Juga: catatan yang sudah ada saat rantai dinyalakan hanya dijamin sebagaimana keadaannya pada hari itu **[L9]**, dan pengunduhan file
ekspor belum dicatat dalam log **[L10]**.

## Kata yang dipakai dan yang dihindari

Pakai: "ketahuan kalau diubah", "dijangkarkan di luar aplikasi", "Anda bisa memeriksanya sendiri". Hindari: "anti-manipulasi", "tidak bisa
diubah", "tidak perlu percaya kami". Lihat [claims.md](claims.md).
