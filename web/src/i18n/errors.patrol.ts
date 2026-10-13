/**
 * Error codes of floor plans, patrol schedules, QR regeneration, exports and
 * paging (D-037, D-038, D-045, D-048). Merged into `ERROR_CODES`.
 */
type Pair = { en: string; id: string }

export const PATROL_ERROR_CODES: Record<string, Pair> = {
  // ── exports & paging (D-045) ─────────────────────────────────────────────
  export_too_large: {
    en: 'The export is too large. Narrow the date range or the filters and try again.',
    id: 'Ekspor terlalu besar. Persempit rentang tanggal atau filter lalu coba lagi.',
  },
  invalid_format: { en: 'This download format is not available for this report.', id: 'Format unduhan ini tidak tersedia untuk laporan ini.' },
  invalid_limit: { en: 'Invalid page size.', id: 'Ukuran halaman tidak valid.' },
  // ── floor plans (D-048) ──────────────────────────────────────────────────
  floorplan_in_use: {
    en: 'Checkpoints are still placed on this floor plan. Move them to another plan (or remove their position) in the route editor first.',
    id: 'Masih ada titik yang ditempatkan di denah ini. Pindahkan dulu ke denah lain (atau hapus posisinya) di editor rute.',
  },
  too_many_floorplans: { en: 'A site can have at most 20 floor plans. Delete an unused one first.', id: 'Satu lokasi maksimal 20 denah. Hapus denah yang tidak dipakai terlebih dahulu.' },
  invalid_plan_position: {
    en: 'A checkpoint position on the floor plan is not valid (it must lie on the image).',
    id: 'Posisi titik pada denah tidak valid (harus berada di dalam gambar).',
  },
  unknown_floorplan: { en: 'The floor plan no longer exists. Reload and place the checkpoint again.', id: 'Denah sudah tidak ada. Muat ulang lalu tempatkan titik lagi.' },
  // ── patrol schedules (D-038) ─────────────────────────────────────────────
  end_time_required: { en: 'A repeating window needs an end time.', id: 'Jadwal berulang perlu jam selesai.' },
  invalid_every: { en: 'Repeat every 15 to 720 minutes.', id: 'Ulangi setiap 15 sampai 720 menit.' },
  invalid_grace: { en: 'The grace period must be 0 to 180 minutes.', id: 'Toleransi harus 0 sampai 180 menit.' },
  grace_too_long: { en: 'The grace period can be at most half of the interval.', id: 'Toleransi maksimal setengah dari selang waktu.' },
  too_many_schedules: { en: 'A route can have at most 20 schedule windows.', id: 'Satu rute maksimal 20 jadwal.' },
  // ── incidents (D-049) ────────────────────────────────────────────────────
  invalid_gps: { en: 'The GPS position is not valid.', id: 'Posisi GPS tidak valid.' },
}

/** Numbers of an `export_too_large` message ("… has 61234 rows; the limit for csv is 50000 …"). */
export function exportLimit(message: string | undefined): { rows?: number; limit?: number; format?: string } {
  const m = /has (\d+) rows?; the limit for (\w+) is (\d+)/i.exec(message ?? '')
  return m ? { rows: Number(m[1]), format: m[2], limit: Number(m[3]) } : {}
}

const TOO_LARGE: Pair = {
  en: 'The export has {rows} rows, more than the {format} limit of {limit}. Narrow the date range or the filters and try again.',
  id: 'Ekspor berisi {rows} baris, melebihi batas {format} sebesar {limit}. Persempit rentang tanggal atau filter lalu coba lagi.',
}

/** Translated `export_too_large` text with the limit, when the message carries it. */
export function exportTooLargeText(message: string | undefined, lang: 'en' | 'id'): string {
  const { rows, limit, format } = exportLimit(message)
  if (rows === undefined || limit === undefined) return PATROL_ERROR_CODES.export_too_large[lang]
  const n = (v: number) => new Intl.NumberFormat(lang === 'id' ? 'id-ID' : 'en-GB').format(v)
  return TOO_LARGE[lang]
    .replace('{rows}', n(rows))
    .replace('{limit}', n(limit))
    .replace('{format}', (format ?? '').toUpperCase())
}
