import type { proofpackEn } from './proofpack.en'

export const proofpackId: Record<keyof typeof proofpackEn, string> = {
  'pp.title': 'Paket bukti',
  'pp.q': 'PDF yang bisa disimpan dan diperiksa klien: bulan itu dalam angka, ditambah hash rantai dan tanda tangan.',
  'pp.who': 'Admin · Supervisor',
  'pp.intro': 'Paket bukti memuat bulan {site}: kehadiran, patroli, insiden, dan tanda peringatan, per hari dan per Satpam, dengan hash rantai catatan dan tanda tangan terbaru. Halaman terakhirnya menjelaskan cara memeriksanya dengan garda-verify.',
  'pp.month': 'Bulan',
  'pp.lang': 'Bahasa',
  'pp.langSite': 'Bahasa lokasi ({lang})',
  'pp.pdf': 'Unduh paket bukti (PDF)',
  'pp.pdfHint': 'Dibuat sekarang dari catatan terkini.',
  'pp.kept': 'Unduh salinan tersimpan',
  'pp.keptHint': 'Salinan yang dibuat sistem pada hari-hari pertama bulan.',
  'pp.bundle': 'Unduh berkas ekspor (ZIP)',
  'pp.bundleHint': 'Berkas yang diperiksa pembaca dengan garda-verify. Isinya catatan bulan itu, jadi bagikan hanya kepada yang boleh melihatnya.',
  'pp.unsigned': 'Bila PDF menulis UNSIGNED TAIL, belum ada tanda tangan setelah bulan berakhir yang mencakup catatan terakhir. Buat lagi setelah tanda tangan berikutnya; tidak pernah ditampilkan sebagai terverifikasi penuh.',
  'pp.openMonth': 'Bulan ini belum selesai, jadi paket belum bisa ditandatangani setelah akhir bulan.',
}
