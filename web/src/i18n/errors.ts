import { apiErrorCode, apiErrorMessage } from '../api/client'
import type { Lang } from '../lib/time'

/**
 * Translations for backend error codes. Unknown codes fall back to the server
 * message (`error` field), which is English.
 */
const codes: Record<string, { en: string; id: string }> = {
  not_found: { en: 'Not found.', id: 'Data tidak ditemukan.' },
  duplicate: { en: 'This already exists.', id: 'Data ini sudah ada.' },
  invalid_reference: { en: 'A referenced record does not exist.', id: 'Data yang dirujuk tidak ada.' },
  site_id_required: { en: 'Choose a site.', id: 'Pilih lokasi.' },
  shift_has_attendance: {
    en: 'This shift already has attendance — use an attendance override instead.',
    id: 'Shift ini sudah memiliki kehadiran — gunakan koreksi kehadiran.',
  },
  guard_other_site: { en: 'The guard is not assigned to this site.', id: 'Petugas tidak ditugaskan di lokasi ini.' },
  weak_pin: { en: 'PIN is too easy to guess.', id: 'PIN terlalu mudah ditebak.' },
  invalid_pin: { en: 'PIN must be 4–6 digits.', id: 'PIN harus 4–6 angka.' },
  site_required: {
    en: 'Assign a site before activating the tablet.',
    id: 'Pilih lokasi sebelum mengaktifkan tablet.',
  },
  reason_required: { en: 'A reason is required.', id: 'Alasan wajib diisi.' },
  range_too_long: { en: 'The date range is too long.', id: 'Rentang tanggal terlalu panjang.' },
  invalid_range: { en: "'To' must not be before 'from'.", id: "'Sampai' tidak boleh sebelum 'dari'." },
  not_failed: { en: 'Only failed deliveries can be retried.', id: 'Hanya pengiriman gagal yang dapat dikirim ulang.' },
}

const generic = { en: 'Something went wrong.', id: 'Terjadi kesalahan.' }
const forbidden = { en: 'You are not allowed to do this.', id: 'Anda tidak diizinkan melakukan ini.' }

/** Human-readable message for an API error in the current language. */
export function errorText(err: unknown, lang: Lang, fallbackMessage?: string): string {
  const code = apiErrorCode(err)
  if (code && codes[code]) return codes[code][lang]
  const msg = fallbackMessage ?? apiErrorMessage(err)
  if (msg) return msg
  if (typeof err === 'object' && err && 'response' in err) {
    const status = (err as { response?: { status?: number } }).response?.status
    if (status === 403) return forbidden[lang]
  }
  return generic[lang]
}
