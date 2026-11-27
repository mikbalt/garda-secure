package proofpack

// words are the pack's texts in one language. "Satpam" is the word for a
// guard in Indonesian; plain words, no jargon (the reader is a client, not an
// engineer). The "does NOT prove" list repeats docs/chain-verification.md.
type words struct {
	title, site, period, zone, generated, language string
	monthOpen                                      string // %s = generated at

	// Banners; %s = detail.
	signed, unsignedTail, failed, notProven, noActivity, notChecked string

	summary, attendance, patrol, incidents, flags string
	attLine, attShifts                            string // guard-days, shifts
	patLine, patIssues                            string
	incLine, incNone, incBy                       string
	flagsIntro, flagsNone, flagsMore              string
	flagCols                                      [3]string

	perDay, perGuard string
	dayCols          [6]string
	guardCols        [8]string
	total            string
	perDayNote       string

	chain                                                              string
	chainEntries, chainTip, chainHead, chainSeq, chainSigned, chainKey string
	chainSig, chainHeadHash, chainNoHead, chainFp, chainChecked        string
	chainNote                                                          string

	howTo                      string
	howIntro                   string
	step1, step2, step3, step4 string
	cmdNote                    string
	codesTitle                 string
	codes                      []string
	limitsTitle                string
	limits                     []string
	unsignedAgain              string
	reasonNoHead, reasonTail   string
	reasonOld, reasonOther     string
}

var en = words{
	title: "Monthly proof pack", site: "Site", period: "Period", zone: "Time zone", generated: "Generated", language: "Language",
	monthOpen: "This month is not over yet. The pack covers the days up to %s only.",

	signed:       "CHAIN CHECK PASSED when this pack was made. GARDA's own server found no break in the record chain of this month, and a signature made after the month ended covers the last record. This is the server checking itself, so check it yourself with the steps on the last page.",
	unsignedTail: "UNSIGNED TAIL. No signature made after the end of this month covers the last records of the month (%s). Do NOT treat this pack as fully verified. Make it again after the next signature (the signer signs every hour).",
	failed:       "CHAIN CHECK FAILED. %s. Do not rely on this pack. Show it to your GARDA administrator.",
	notProven:    "NOT FULLY VERIFIED. %s",
	noActivity:   "NO ACTIVITY RECORDED. GARDA holds no record for this site in this month, so there is nothing to sign. The figures below are zero. This pack cannot prove that nothing happened: a record that was never made looks the same.",
	notChecked:   "the chain could not be read when this pack was made",

	summary: "Summary", attendance: "Attendance", patrol: "Patrols", incidents: "Incidents", flags: "Warning flags",
	attLine:   "Guard-days (one day of one guard counts once): %d scheduled, %d present, %d late, %d absent. Hours worked: %s.",
	attShifts: "Shifts: %d scheduled, %d present, %d late, %d absent, %d not decided yet.",
	patLine:   "Patrol rounds started: %d. On average %s%% of the checkpoints were scanned.",
	patIssues: "Missed time limits (SLA): %d. Scans with a warning flag: %d.",
	incLine:   "Incidents reported: %d.", incNone: "No incident reported.", incBy: "By severity: %s.",
	flagsIntro: "Automatic warnings about records that look odd (for example a late upload or a position far from the checkpoint). A warning is a reason to look, not proof of wrongdoing.",
	flagsNone:  "No warning flags this month.",
	flagsMore:  "More than %d flags: only the first %d are counted.",
	flagCols:   [3]string{"Flag", "Count", "Reviewed"},

	perDay: "Per day", perGuard: "Per guard",
	dayCols:    [6]string{"Date", "Present", "Late", "Absent", "Patrol rounds", "Incidents"},
	guardCols:  [8]string{"Guard", "Days", "Present", "Late", "Absent", "Hours", "Patrol rounds / %", "Incidents"},
	total:      "Total",
	perDayNote: "Counts of shifts by outcome (site time zone; a night shift counts on the day it starts). Names and counts only: no photos, no incident text.",

	chain:        "Chain proof",
	chainEntries: "Records chained this month", chainTip: "Chain head when made", chainHead: "Newest signed head",
	chainSeq: "Entry number (seq)", chainSigned: "Signed at (UTC)", chainKey: "Signer key (fingerprint)",
	chainSig: "Signature", chainHeadHash: "Entry hash", chainNoHead: "None: no signature covers this month yet.",
	chainFp: "Key fingerprint to pin", chainChecked: "Server's own check",
	chainNote: "Every record GARDA keeps (attendance, patrol scans, incidents) is linked to the one before it by a hash, so one change breaks every link after it. A separate signer, on its own machine, signs the newest link from time to time. Change or delete a record later and the links no longer match the signature.",

	howTo:      "How to check this pack yourself",
	howIntro:   "You do not need to trust this PDF or GARDA's server. The steps below check the records with a small program on your own computer.",
	step1:      "1. Ask your GARDA administrator for the export file of this month (in the GARDA web app: Reports, Proof pack, Download export bundle). Save it as:",
	step2:      "2. Get the program garda-verify from your GARDA administrator (one file, no installation, works offline).",
	step3:      "3. Run it with the key fingerprint you were given when GARDA was installed. Compare it with the fingerprint printed in this pack; if they differ, stop and ask. Command:",
	step4:      "4. Read the last line it prints. Exit code 0 and the word VALID mean the records in the file are linked and signed. Any other code means something is open or wrong:",
	cmdNote:    "",
	codesTitle: "",
	codes: []string{
		"0 = VALID. See the list below: VALID does not mean everything is proven.",
		"7 = UNSIGNED TAIL: the newest records are not signed yet. Export again after the next signature.",
		"8 or 12 = some records changed again after the file was made, so their text could not be checked.",
		"9 = the file holds no record. 10 = the file does not cover the month. 11 = the month was not given to the program.",
		"1, 3, 4, 5, 6 = something does not match: a record, the file, a signature, the key, or the signature is missing or old. Do not rely on the pack; ask your GARDA administrator.",
	},
	limitsTitle: "What VALID does NOT prove",
	limits: []string{
		"That nothing is missing. A file that ends early can still be valid if it ends exactly at a signature. Check that the number of records looks right for the month.",
		"That the dates are the ones you asked for. Only --expect-from and --expect-to check them against the signing times.",
		"Records that changed again outside the file: their text is not checked (exit code 8).",
		"That the file is current. An old, genuine file stays valid for ever. Look at the export time and the newest signature.",
		"That nobody with full control of the server and of the signer's machine acted. They could sign a rewritten history. Pinning the fingerprint, keeping your own earlier files and handing over keys in your presence limit this.",
		"That a site with no records had no activity. An empty file is exit code 9, not valid.",
		"That nobody saw the data. Exports are not logged yet and the file holds personal data.",
		"That the file hashes inside the file protect it. They only show damage or a careless edit. The chain and the signatures protect it.",
	},
	reasonNoHead: "no signature at all yet", reasonTail: "records after the newest signature", reasonOld: "the newest signature is older than the end of the month", reasonOther: "the signatures do not cover the whole month",
	unsignedAgain: "This pack has an UNSIGNED TAIL: run the check again on a file exported after the next signature. Exit code 7 is expected until then.",
}

var id = words{
	title: "Paket bukti bulanan", site: "Lokasi", period: "Periode", zone: "Zona waktu", generated: "Dibuat", language: "Bahasa",
	monthOpen: "Bulan ini belum selesai. Paket ini hanya mencakup hari sampai %s.",

	signed:       "PEMERIKSAAN RANTAI LOLOS saat paket ini dibuat. Server GARDA sendiri tidak menemukan rantai catatan bulan ini yang putus, dan tanda tangan yang dibuat setelah bulan berakhir mencakup catatan terakhir. Ini server yang memeriksa dirinya sendiri, jadi periksa sendiri dengan langkah di halaman terakhir.",
	unsignedTail: "UNSIGNED TAIL (EKOR BELUM DITANDATANGANI). Belum ada tanda tangan setelah akhir bulan ini yang mencakup catatan terakhir bulan ini (%s). JANGAN anggap paket ini sudah terverifikasi penuh. Buat ulang setelah tanda tangan berikutnya (penanda tangan menandatangani setiap jam).",
	failed:       "PEMERIKSAAN RANTAI GAGAL. %s. Jangan andalkan paket ini. Tunjukkan kepada administrator GARDA Anda.",
	notProven:    "BELUM TERVERIFIKASI PENUH. %s",
	noActivity:   "TIDAK ADA AKTIVITAS TERCATAT. GARDA tidak menyimpan catatan apa pun untuk lokasi ini di bulan ini, jadi tidak ada yang ditandatangani. Angka di bawah semuanya nol. Paket ini tidak bisa membuktikan bahwa tidak ada kejadian: catatan yang tidak pernah dibuat terlihat sama.",
	notChecked:   "rantai tidak dapat dibaca saat paket ini dibuat",

	summary: "Ringkasan", attendance: "Kehadiran", patrol: "Patroli", incidents: "Insiden", flags: "Tanda peringatan",
	attLine:   "Hari-Satpam (satu hari satu Satpam dihitung sekali): %d terjadwal, %d hadir, %d terlambat, %d absen. Jam kerja: %s.",
	attShifts: "Shift: %d terjadwal, %d hadir, %d terlambat, %d absen, %d belum diputuskan.",
	patLine:   "Putaran patroli dimulai: %d. Rata-rata %s%% titik pemeriksaan terpindai.",
	patIssues: "Batas waktu terlewat (SLA): %d. Pindaian dengan tanda peringatan: %d.",
	incLine:   "Insiden dilaporkan: %d.", incNone: "Tidak ada insiden dilaporkan.", incBy: "Menurut tingkat keparahan: %s.",
	flagsIntro: "Peringatan otomatis untuk catatan yang tampak janggal (misalnya diunggah terlambat atau posisi jauh dari titik pemeriksaan). Peringatan adalah alasan untuk memeriksa, bukan bukti kesalahan.",
	flagsNone:  "Tidak ada tanda peringatan bulan ini.",
	flagsMore:  "Lebih dari %d tanda: hanya %d pertama yang dihitung.",
	flagCols:   [3]string{"Tanda", "Jumlah", "Ditinjau"},

	perDay: "Per hari", perGuard: "Per Satpam",
	dayCols:    [6]string{"Tanggal", "Hadir", "Terlambat", "Absen", "Putaran patroli", "Insiden"},
	guardCols:  [8]string{"Satpam", "Hari", "Hadir", "Terlambat", "Absen", "Jam", "Putaran patroli / %", "Insiden"},
	total:      "Total",
	perDayNote: "Jumlah shift menurut hasilnya (zona waktu lokasi; shift malam dihitung pada hari mulainya). Hanya nama dan jumlah: tanpa foto, tanpa isi laporan insiden.",

	chain:        "Bukti rantai",
	chainEntries: "Catatan terantai bulan ini", chainTip: "Ujung rantai saat dibuat", chainHead: "Tanda tangan terbaru",
	chainSeq: "Nomor entri (seq)", chainSigned: "Ditandatangani (UTC)", chainKey: "Kunci penanda tangan (sidik jari)",
	chainSig: "Tanda tangan", chainHeadHash: "Hash entri", chainNoHead: "Tidak ada: belum ada tanda tangan yang mencakup bulan ini.",
	chainFp: "Sidik jari kunci yang dipatok", chainChecked: "Pemeriksaan server sendiri",
	chainNote: "Setiap catatan yang disimpan GARDA (kehadiran, pindaian patroli, insiden) disambung ke catatan sebelumnya dengan hash, sehingga satu perubahan memutus semua sambungan sesudahnya. Penanda tangan terpisah, di mesinnya sendiri, menandatangani sambungan terbaru dari waktu ke waktu. Bila catatan diubah atau dihapus kemudian, sambungan tidak lagi cocok dengan tanda tangan.",

	howTo:    "Cara memeriksa paket ini sendiri",
	howIntro: "Anda tidak perlu percaya pada PDF ini atau server GARDA. Langkah di bawah memeriksa catatan dengan program kecil di komputer Anda sendiri.",
	step1:    "1. Minta administrator GARDA Anda berkas ekspor bulan ini (di aplikasi web GARDA: Laporan, Paket bukti, Unduh berkas ekspor). Simpan dengan nama:",
	step2:    "2. Minta program garda-verify dari administrator GARDA Anda (satu berkas, tanpa instalasi, jalan tanpa internet).",
	step3:    "3. Jalankan dengan sidik jari kunci yang Anda terima saat GARDA dipasang. Bandingkan dengan sidik jari yang tercetak di paket ini; bila berbeda, berhenti dan tanyakan. Perintah:",
	step4:    "4. Baca baris terakhir yang dicetaknya. Kode keluar 0 dan kata VALID berarti catatan di berkas tersambung dan bertanda tangan. Kode lain berarti ada yang belum selesai atau salah:",
	codes: []string{
		"0 = VALID. Lihat daftar di bawah: VALID tidak berarti semuanya terbukti.",
		"7 = UNSIGNED TAIL: catatan terbaru belum ditandatangani. Ekspor lagi setelah tanda tangan berikutnya.",
		"8 atau 12 = sebagian catatan berubah lagi setelah berkas dibuat, sehingga isinya tidak bisa diperiksa.",
		"9 = berkas tidak berisi catatan. 10 = berkas tidak mencakup bulan ini. 11 = bulan tidak diberikan ke program.",
		"1, 3, 4, 5, 6 = ada yang tidak cocok: catatan, berkas, tanda tangan, kunci, atau tanda tangan hilang atau lama. Jangan andalkan paket ini; tanyakan administrator GARDA Anda.",
	},
	limitsTitle: "Yang TIDAK dibuktikan VALID",
	limits: []string{
		"Bahwa tidak ada yang hilang. Berkas yang berakhir lebih awal tetap bisa valid bila berakhir tepat di tanda tangan. Periksa apakah jumlah catatan wajar untuk bulan ini.",
		"Bahwa tanggalnya sesuai permintaan Anda. Hanya --expect-from dan --expect-to yang memeriksanya terhadap waktu tanda tangan.",
		"Catatan yang berubah lagi di luar berkas: isinya tidak diperiksa (kode keluar 8).",
		"Bahwa berkas masih terkini. Berkas lama yang asli tetap valid selamanya. Lihat waktu ekspor dan tanda tangan terbaru.",
		"Bahwa tidak ada pihak dengan kendali penuh atas server dan mesin penanda tangan yang bertindak. Mereka bisa menandatangani riwayat yang ditulis ulang. Mematok sidik jari, menyimpan berkas lama Anda sendiri, dan serah terima kunci di hadapan Anda membatasi hal ini.",
		"Bahwa lokasi tanpa catatan memang tanpa aktivitas. Berkas kosong berkode keluar 9, bukan valid.",
		"Bahwa tidak ada yang melihat datanya. Ekspor belum dicatat dan berkas berisi data pribadi.",
		"Bahwa hash berkas di dalam berkas melindunginya. Itu hanya menunjukkan kerusakan atau suntingan ceroboh. Rantai dan tanda tangan yang melindunginya.",
	},
	reasonNoHead: "belum ada tanda tangan sama sekali", reasonTail: "ada catatan setelah tanda tangan terbaru", reasonOld: "tanda tangan terbaru lebih tua dari akhir bulan", reasonOther: "tanda tangan tidak mencakup seluruh bulan",
	unsignedAgain: "Paket ini punya UNSIGNED TAIL: jalankan pemeriksaan lagi dengan berkas yang diekspor setelah tanda tangan berikutnya. Kode keluar 7 wajar sampai saat itu.",
}

func wordsFor(lang string) *words {
	if lang == "en" {
		return &en
	}
	return &id
}

// tailReason says in plain words why no signature covers the end of the month.
func (w *words) tailReason(verifyStatus string) string {
	switch verifyStatus {
	case "unsigned":
		return w.reasonNoHead
	case "unsigned_tail", "stale":
		return w.reasonTail
	case "range_mismatch":
		return w.reasonOld
	}
	return w.reasonOther
}
