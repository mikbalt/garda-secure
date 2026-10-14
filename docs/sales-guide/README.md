# Panduan Sales GARDA

Panduan untuk tim sales & presales: cerita produk, alur demo 20 menit, tur fitur
dengan screenshot, pertanyaan & keberatan, checklist calon klien, kebutuhan klien,
dan batasan versi 1.

| Format | File | Untuk |
|---|---|---|
| **PDF** | [GARDA-Panduan-Sales.pdf](GARDA-Panduan-Sales.pdf) | Dikirim lewat WhatsApp/e-mail, dicetak. Di GitHub klik file lalu **Download**. |
| **HTML** | [index.html](index.html) + folder [`img/`](img/) | Dibuka di browser (gambar bisa diperbesar). GitHub hanya menampilkan kodenya: unduh repo (**Code → Download ZIP**) atau clone, lalu buka `docs/sales-guide/index.html`. |

## Memperbarui (tim teknis)

Gambar diambil otomatis dari **mode demo** (data contoh), jadi bisa dibuat ulang
setiap kali tampilan berubah:

```bash
cd web
npm ci
PW_CHANNEL=chrome npm run guide:shots   # 43 screenshot web -> docs/sales-guide/img/
PW_CHANNEL=chrome npm run guide:pdf     # index.html -> GARDA-Panduan-Sales.pdf
```

Screenshot tablet berasal dari golden test Flutter
(`cd mobile && flutter test screenshots/ --update-goldens`, lalu salin dari
`mobile/screenshots/out/`; daftar nama file ada di `img/manifest.json`).
`PW_CHANNEL=chrome` memakai Google Chrome yang terpasang; tanpa itu Playwright
memakai Chromium bawaannya (`npx playwright install chromium`).
