import axios from 'axios'
import {
  apiErrorCode,
  apiErrorField,
  apiErrorMessage,
  apiErrorReason,
  apiErrorStatus,
  apiRequestId,
  apiRetryAfter,
  type ApiErrorBody,
} from '../api/client'
import type { Lang } from '../lib/time'
import { PATROL_ERROR_CODES, exportTooLargeText } from './errors.patrol'

type Pair = { en: string; id: string }

/** Codes of /users and /device-accounts (docs/api.md, D-041). */
const USER_ERROR_CODES: Record<string, Pair> = {
  kc_unavailable: {
    en: 'User management is unavailable right now: the login server (Keycloak) cannot be reached or is not set up for this organisation. Try again later or ask the operator.',
    id: 'Manajemen pengguna sedang tidak tersedia: server login (Keycloak) tidak dapat dihubungi atau belum disiapkan untuk organisasi ini. Coba lagi nanti atau hubungi operator.',
  },
  user_not_found: { en: 'User not found.', id: 'Pengguna tidak ditemukan.' },
  username_taken: { en: 'This username is already used.', id: 'Nama pengguna ini sudah dipakai.' },
  email_taken: { en: 'This e-mail address is already used by another user.', id: 'Alamat e-mail ini sudah dipakai pengguna lain.' },
  invalid_username: {
    en: 'Username: 3–64 characters, only a–z, 0–9 and . _ @ - (not starting with "device-").',
    id: 'Nama pengguna: 3–64 karakter, hanya a–z, 0–9 dan . _ @ - (tidak diawali "device-").',
  },
  invalid_email: { en: 'The e-mail address is not valid.', id: 'Alamat e-mail tidak valid.' },
  email_required: { en: 'An e-mail address is needed to send an e-mail.', id: 'Alamat e-mail diperlukan untuk mengirim e-mail.' },
  roles_required: { en: 'Choose at least one role.', id: 'Pilih minimal satu peran.' },
  role_not_allowed: {
    en: 'You may not assign this role, or change a platform operator account.',
    id: 'Anda tidak boleh memberikan peran ini, atau mengubah akun operator platform.',
  },
  cannot_modify_self: {
    en: 'You cannot do this to your own account (remove your admin role, disable it, change your locations, or reset your own password/authenticator here).',
    id: 'Anda tidak dapat melakukan ini pada akun sendiri (mencabut peran admin, menonaktifkan, mengubah lokasi, atau mereset kata sandi/autentikator sendiri di sini).',
  },
  sites_required: {
    en: 'You can only manage users limited to your own locations — choose at least one.',
    id: 'Anda hanya dapat mengelola pengguna yang dibatasi ke lokasi Anda — pilih minimal satu.',
  },
  invalid_site: { en: 'One of the chosen locations does not belong to your organisation.', id: 'Salah satu lokasi yang dipilih bukan milik organisasi Anda.' },
  password_policy: {
    en: "The password does not meet the organisation's password rules. Choose a longer or stronger one, or leave it empty to generate one.",
    id: 'Kata sandi tidak memenuhi aturan kata sandi organisasi. Pilih yang lebih panjang/kuat, atau kosongkan agar dibuat otomatis.',
  },
  invalid_user: { en: 'The login server rejected the change.', id: 'Server login menolak perubahan ini.' },
  invite_failed: {
    en: 'The invitation e-mail could not be sent. Set a temporary password instead.',
    id: 'E-mail undangan tidak dapat dikirim. Atur kata sandi sementara sebagai gantinya.',
  },
  invalid_first: { en: 'Invalid page.', id: 'Halaman tidak valid.' },
  invalid_max: { en: 'Invalid page size.', id: 'Ukuran halaman tidak valid.' },
  invalid_enabled: { en: 'Invalid status filter.', id: 'Filter status tidak valid.' },
}

/**
 * Translations for every backend error code in docs/api.md (WEB-7).
 * Placeholders: {field}, {max}, {seconds}. Unknown codes fall back to a
 * status-based message (Indonesian) or the server message (English).
 */
export const ERROR_CODES: Record<string, Pair> = {
  // ── generic ──────────────────────────────────────────────────────────────
  unauthorized: { en: 'Your session has expired. Please sign in again.', id: 'Sesi Anda sudah berakhir. Silakan masuk lagi.' },
  forbidden: { en: 'You are not allowed to do this.', id: 'Anda tidak diizinkan melakukan ini.' },
  not_found: { en: 'Not found.', id: 'Data tidak ditemukan.' },
  duplicate: { en: 'This already exists.', id: 'Data ini sudah ada.' },
  invalid_reference: { en: 'A referenced record does not exist.', id: 'Data yang dirujuk tidak ada.' },
  invalid_body: { en: 'The request could not be read. Check the form and try again.', id: 'Permintaan tidak dapat dibaca. Periksa isian lalu coba lagi.' },
  invalid_value: { en: 'One of the values is not valid.', id: 'Salah satu nilai tidak valid.' },
  invalid_id: { en: 'The ID is not valid.', id: 'ID tidak valid.' },
  invalid_field: { en: 'This field cannot be changed.', id: 'Kolom ini tidak dapat diubah.' },
  id_conflict: { en: 'This ID already belongs to another record.', id: 'ID ini sudah dipakai data lain.' },
  field_too_long: { en: '{field} is too long (max. {max} characters).', id: '{field} terlalu panjang (maks. {max} karakter).' },
  body_too_large: { en: 'The data sent is too large.', id: 'Data yang dikirim terlalu besar.' },
  rate_limited: {
    en: 'Too many requests. Please wait {seconds} seconds and try again.',
    id: 'Terlalu banyak permintaan. Tunggu {seconds} detik lalu coba lagi.',
  },
  platform_admin_required: {
    en: 'Only the platform operator (role platform-admin) can do this.',
    id: 'Hanya operator platform (peran platform-admin) yang dapat melakukan ini.',
  },
  internal: {
    en: 'Something went wrong on the server. Please try again.',
    id: 'Terjadi kesalahan di server. Silakan coba lagi.',
  },
  other_site: { en: 'This record belongs to another site.', id: 'Data ini milik lokasi lain.' },
  timestamp_out_of_range: { en: 'The time is outside the allowed window.', id: 'Waktunya di luar rentang yang diizinkan.' },
  not_your_record: { en: 'This record belongs to someone else.', id: 'Data ini milik orang lain.' },
  // ── users & tablet accounts (F1, D-041) ─────────────────────────────────
  ...USER_ERROR_CODES,
  // ── sites ────────────────────────────────────────────────────────────────
  name_required: { en: 'A name is required.', id: 'Nama wajib diisi.' },
  invalid_name: { en: 'The name is not valid.', id: 'Nama tidak valid.' },
  invalid_timezone: { en: 'Unknown time zone.', id: 'Zona waktu tidak dikenal.' },
  invalid_config: { en: 'One of the site settings is out of range.', id: 'Salah satu pengaturan lokasi di luar batas.' },
  site_id_required: { en: 'Choose a site.', id: 'Pilih lokasi.' },
  site_required: { en: 'Assign a site before activating the tablet.', id: 'Pilih lokasi sebelum mengaktifkan tablet.' },
  // ── guards & devices ─────────────────────────────────────────────────────
  employee_id_taken: {
    en: 'This employee ID is already used by another guard in your organisation.',
    id: 'NIK/ID karyawan ini sudah dipakai petugas lain di organisasi Anda.',
  },
  guard_not_found: { en: 'Guard not found.', id: 'Petugas tidak ditemukan.' },
  guard_other_site: { en: 'The guard is not assigned to this site.', id: 'Petugas tidak ditugaskan di lokasi ini.' },
  invalid_role: { en: 'Unknown role.', id: 'Peran tidak dikenal.' },
  invalid_status: { en: 'Unknown status.', id: 'Status tidak dikenal.' },
  weak_pin: { en: 'PIN is too easy to guess.', id: 'PIN terlalu mudah ditebak.' },
  invalid_pin: { en: 'PIN must be 4–6 digits.', id: 'PIN harus 4–6 angka.' },
  device_inactive: { en: 'This tablet is not active.', id: 'Tablet ini tidak aktif.' },
  device_unregistered: { en: 'This tablet is not registered.', id: 'Tablet ini belum terdaftar.' },
  device_pending: { en: 'This tablet is waiting for approval.', id: 'Tablet ini menunggu persetujuan.' },
  device_revoked: { en: 'Access for this tablet was revoked.', id: 'Akses tablet ini sudah dicabut.' },
  guard_unknown: { en: 'Unknown guard.', id: 'Petugas tidak dikenal.' },
  guard_not_allowed: { en: 'This guard may not use this tablet.', id: 'Petugas ini tidak boleh memakai tablet ini.' },
  guard_required: { en: 'A guard must be signed in on the tablet.', id: 'Petugas harus masuk di tablet.' },
  supervisor_only: { en: 'Only a supervisor can do this.', id: 'Hanya supervisor yang dapat melakukan ini.' },
  // ── schedules ────────────────────────────────────────────────────────────
  invalid_start_time: { en: 'The start time is not valid.', id: 'Jam mulai tidak valid.' },
  invalid_end_time: { en: 'The end time is not valid.', id: 'Jam selesai tidak valid.' },
  invalid_weekdays: { en: 'Choose at least one weekday.', id: 'Pilih minimal satu hari.' },
  invalid_valid_from: { en: "The 'valid from' date is not valid.", id: "Tanggal 'berlaku dari' tidak valid." },
  invalid_valid_until: { en: "The 'valid until' date must be after 'valid from'.", id: "Tanggal 'berlaku sampai' harus setelah 'berlaku dari'." },
  invalid_times: { en: 'The end must be after the start.', id: 'Waktu selesai harus setelah waktu mulai.' },
  times_required: { en: 'Start and end times are required.', id: 'Waktu mulai dan selesai wajib diisi.' },
  invalid_time: { en: 'The time is not valid.', id: 'Waktu tidak valid.' },
  invalid_date: { en: 'The date is not valid.', id: 'Tanggal tidak valid.' },
  invalid_month: { en: 'The month is not valid.', id: 'Bulan tidak valid.' },
  invalid_offset: { en: 'Invalid page.', id: 'Halaman tidak valid.' },
  shift_has_attendance: {
    en: 'This shift already has attendance — use an attendance override instead.',
    id: 'Shift ini sudah memiliki kehadiran — gunakan koreksi kehadiran.',
  },
  range_too_long: { en: 'The date range is too long.', id: 'Rentang tanggal terlalu panjang.' },
  invalid_range: { en: "'To' must not be before 'from'.", id: "'Sampai' tidak boleh sebelum 'dari'." },
  // ── attendance ───────────────────────────────────────────────────────────
  no_shift: { en: 'There is no shift to clock in to.', id: 'Tidak ada shift untuk absen masuk.' },
  already_clocked_in: { en: 'Already clocked in for this shift.', id: 'Sudah absen masuk untuk shift ini.' },
  not_clocked_in: { en: 'Not clocked in.', id: 'Belum absen masuk.' },
  clock_out_window_closed: { en: 'The clock-out window has closed.', id: 'Batas waktu absen pulang sudah lewat.' },
  attendance_not_found: { en: 'Attendance record not found.', id: 'Data kehadiran tidak ditemukan.' },
  before_clock_in: { en: 'Clock-out cannot be before clock-in.', id: 'Absen pulang tidak boleh sebelum absen masuk.' },
  reason_required: { en: 'A reason is required.', id: 'Alasan wajib diisi.' },
  // ── patrol ───────────────────────────────────────────────────────────────
  checkpoints_required: { en: 'A route needs at least one checkpoint.', id: 'Rute perlu minimal satu titik.' },
  checkpoint_name_required: { en: 'Every checkpoint needs a name.', id: 'Setiap titik wajib diberi nama.' },
  checkpoint_required: { en: 'Choose a checkpoint.', id: 'Pilih titik.' },
  duplicate_checkpoint: { en: 'A checkpoint appears twice.', id: 'Ada titik yang tercantum dua kali.' },
  duplicate_qr_code: { en: 'Two checkpoints have the same QR code.', id: 'Dua titik memiliki kode QR yang sama.' },
  too_many_checkpoints: { en: 'Too many checkpoints (max. 500).', id: 'Terlalu banyak titik (maks. 500).' },
  invalid_sla: { en: 'The time limit must be a positive number of minutes.', id: 'Batas waktu harus berupa menit positif.' },
  route_inactive: { en: 'This route is inactive.', id: 'Rute ini tidak aktif.' },
  route_not_found: { en: 'Route not found.', id: 'Rute tidak ditemukan.' },
  route_other_site: { en: 'The route belongs to another site.', id: 'Rute ini milik lokasi lain.' },
  session_ended: { en: 'This patrol has already ended.', id: 'Patroli ini sudah selesai.' },
  session_not_found: { en: 'Patrol not found.', id: 'Patroli tidak ditemukan.' },
  session_other_site: { en: 'The patrol belongs to another site.', id: 'Patroli ini milik lokasi lain.' },
  unknown_checkpoint: { en: 'Unknown checkpoint.', id: 'Titik tidak dikenal.' },
  invalid_method: { en: 'Unknown scan method.', id: 'Metode pindai tidak dikenal.' },
  photo_required: { en: 'A photo is required.', id: 'Foto wajib dilampirkan.' },
  before_session_start: { en: 'The time is before the patrol started.', id: 'Waktunya sebelum patroli dimulai.' },
  // ── incidents, defects, visitors ─────────────────────────────────────────
  type_required: { en: 'Choose a category.', id: 'Pilih kategori.' },
  invalid_severity: { en: 'Unknown severity.', id: 'Tingkat keparahan tidak dikenal.' },
  changes_required: { en: 'Change at least one field.', id: 'Ubah minimal satu kolom.' },
  too_many_photos: { en: 'Too many photos (max. 5).', id: 'Terlalu banyak foto (maks. 5).' },
  defect_type_required: { en: 'Choose the kind of defect.', id: 'Pilih jenis kerusakan.' },
  visitor_name_required: { en: "The visitor's name is required.", id: 'Nama tamu wajib diisi.' },
  exit_before_entry: { en: 'Exit cannot be before entry.', id: 'Waktu keluar tidak boleh sebelum waktu masuk.' },
  // ── media ────────────────────────────────────────────────────────────────
  file_required: { en: 'Choose a file.', id: 'Pilih berkas.' },
  file_too_large: { en: 'The photo is too large (max. 10 MB).', id: 'Foto terlalu besar (maks. 10 MB).' },
  invalid_file_type: { en: 'Only JPEG, PNG or WebP photos are accepted.', id: 'Hanya foto JPEG, PNG, atau WebP yang diterima.' },
  invalid_kind: { en: 'Unknown photo kind.', id: 'Jenis foto tidak dikenal.' },
  invalid_object_key: { en: 'The photo reference is not valid.', id: 'Rujukan foto tidak valid.' },
  invalid_image: { en: 'The photo could not be read.', id: 'Foto tidak dapat dibaca.' },
  media_exists: { en: 'This photo was already uploaded and cannot be replaced.', id: 'Foto ini sudah diunggah dan tidak dapat diganti.' },
  media_forbidden: { en: 'Your role may not view this photo.', id: 'Peran Anda tidak boleh melihat foto ini.' },
  // ── notifications & channels ─────────────────────────────────────────────
  invalid_channel: { en: 'Unknown channel.', id: 'Saluran tidak dikenal.' },
  invalid_event_type: { en: 'Unknown event.', id: 'Kejadian tidak dikenal.' },
  invalid_recipient: { en: 'The recipient is not valid.', id: 'Penerima tidak valid.' },
  not_failed: { en: 'Only failed deliveries can be retried.', id: 'Hanya pengiriman gagal yang dapat dikirim ulang.' },
  delivery_failed: { en: 'The test message could not be sent.', id: 'Pesan uji tidak dapat dikirim.' },
  secret_required: {
    en: 'You changed where messages are sent — re-enter the password / token to confirm.',
    id: 'Tujuan pengiriman diubah — masukkan ulang kata sandi / token untuk konfirmasi.',
  },
  invalid_smtp_port: { en: 'SMTP port must be 25, 465, 587 or 2525.', id: 'Port SMTP harus 25, 465, 587, atau 2525.' },
  invalid_smtp_host: { en: 'The SMTP server name is not valid.', id: 'Nama server SMTP tidak valid.' },
  smtp_host_not_allowed: {
    en: 'This SMTP server points to an internal address and is not allowed.',
    id: 'Server SMTP ini mengarah ke alamat internal dan tidak diizinkan.',
  },
  smtp_host_unresolvable: { en: 'The SMTP server name could not be found (DNS).', id: 'Nama server SMTP tidak ditemukan (DNS).' },
  whatsapp_url_not_allowed: {
    en: 'This WhatsApp API URL is not allowed (default: https://graph.facebook.com/).',
    id: 'URL API WhatsApp ini tidak diizinkan (bawaan: https://graph.facebook.com/).',
  },
  invalid_phone_number_id: { en: 'The phone number ID must be digits only.', id: 'Phone number ID hanya boleh berisi angka.' },
  ...PATROL_ERROR_CODES,
}

/** Why a channel test failed (`reason` of 502 `delivery_failed`). */
export const DELIVERY_REASONS: Record<string, Pair> = {
  not_configured: { en: 'The channel is not configured yet.', id: 'Saluran belum diatur.' },
  invalid_recipient: { en: 'The recipient address/number is not valid.', id: 'Alamat/nomor penerima tidak valid.' },
  destination_not_allowed: { en: 'The server address is not allowed.', id: 'Alamat server tidak diizinkan.' },
  timeout: { en: 'The server did not answer in time.', id: 'Server tidak menjawab tepat waktu.' },
  connection_failed: { en: 'Could not connect to the server.', id: 'Tidak dapat terhubung ke server.' },
  tls_failed: { en: 'Secure connection (TLS) failed — check host and port.', id: 'Koneksi aman (TLS) gagal — periksa host dan port.' },
  auth_failed: { en: 'Login was rejected — check user and password / token.', id: 'Login ditolak — periksa pengguna dan kata sandi / token.' },
  rejected: { en: 'The provider rejected the message.', id: 'Penyedia menolak pesan.' },
  failed: { en: 'Sending failed (details in the server log).', id: 'Pengiriman gagal (detail di log server).' },
}

/** Display names of input fields reported by `field_too_long`. */
const FIELD_NAMES: Record<string, Pair> = {
  name: { en: 'Name', id: 'Nama' },
  description: { en: 'Description', id: 'Deskripsi' },
  location: { en: 'Location', id: 'Lokasi' },
  reason: { en: 'Reason', id: 'Alasan' },
  note: { en: 'Note', id: 'Catatan' },
  purpose: { en: 'Purpose', id: 'Keperluan' },
  address: { en: 'Address', id: 'Alamat' },
  employee_id: { en: 'Employee ID', id: 'ID karyawan' },
  id_number: { en: 'ID number', id: 'Nomor identitas' },
  id_type: { en: 'ID type', id: 'Jenis identitas' },
  plate: { en: 'Plate number', id: 'Nomor polisi' },
  type: { en: 'Category', id: 'Kategori' },
  defect_type: { en: 'Defect type', id: 'Jenis kerusakan' },
  visitor_name: { en: "Visitor's name", id: 'Nama tamu' },
  host_name: { en: 'Host', id: 'Yang dikunjungi' },
  recipient: { en: 'Recipient', id: 'Penerima' },
  client_id: { en: 'Client ID', id: 'ID klien' },
  qr_code: { en: 'QR code', id: 'Kode QR' },
  username: { en: 'Username', id: 'Nama pengguna' },
  first_name: { en: 'First name', id: 'Nama depan' },
  last_name: { en: 'Last name', id: 'Nama belakang' },
  email: { en: 'E-mail', id: 'E-mail' },
  temporary_password: { en: 'Temporary password', id: 'Kata sandi sementara' },
}

/** Messages by HTTP status for codes without a translation. */
const BY_STATUS: Record<number, Pair> = {
  400: { en: 'The data is not valid.', id: 'Data tidak valid.' },
  401: ERROR_CODES.unauthorized,
  403: ERROR_CODES.forbidden,
  404: ERROR_CODES.not_found,
  409: { en: 'This conflicts with the current data. Reload and try again.', id: 'Bertentangan dengan data saat ini. Muat ulang lalu coba lagi.' },
  413: ERROR_CODES.body_too_large,
  429: { en: 'Too many requests. Please wait a moment and try again.', id: 'Terlalu banyak permintaan. Tunggu sebentar lalu coba lagi.' },
}

const generic: Pair = { en: 'Something went wrong.', id: 'Terjadi kesalahan.' }
const network: Pair = { en: 'Cannot reach the server. Check your connection.', id: 'Tidak dapat menghubungi server. Periksa koneksi Anda.' }
const reference: Pair = { en: 'Reference for support: {id}', id: 'Kode untuk tim dukungan: {id}' }

function fill(s: string, vars: Record<string, string | number>): string {
  return s.replace(/\{(\w+)\}/g, (m, k: string) => (k in vars ? String(vars[k]) : m))
}

/** Human name of an input field (falls back to the humanised key). */
export function fieldLabel(field: string, lang: Lang): string {
  const known = FIELD_NAMES[field]
  if (known) return known[lang]
  const h = field.replace(/[_.]+/g, ' ').trim()
  return h.charAt(0).toUpperCase() + h.slice(1)
}

/** Translated test-delivery failure reason. */
export function deliveryReasonText(reason: string | undefined, lang: Lang): string | undefined {
  if (!reason) return undefined
  return (DELIVERY_REASONS[reason] ?? DELIVERY_REASONS.failed)[lang]
}

/** "Reference for support: …" line for 5xx answers, or undefined. */
export function requestIdText(err: unknown, lang: Lang): string | undefined {
  const id = apiRequestId(err)
  return id ? fill(reference[lang], { id }) : undefined
}

interface Overrides {
  /** Error body parsed elsewhere (e.g. from a blob download). */
  body?: ApiErrorBody
}

/**
 * Human-readable message for an API error in the current language. Never
 * shows raw server text in Indonesian; 5xx answers carry the request id.
 */
export function errorText(err: unknown, lang: Lang, overrides: Overrides = {}): string {
  const b = overrides.body
  const code = b?.code ?? apiErrorCode(err)
  const status = apiErrorStatus(err)
  const isAxios = axios.isAxiosError(err)
  if (code === undefined && status === 401) return ERROR_CODES.unauthorized[lang]

  // 5xx answers are generic — except codes that tell the user what to do (503 kc_unavailable).
  if (status !== undefined && status >= 500 && code !== 'delivery_failed' && code !== 'kc_unavailable') {
    const base = ERROR_CODES.internal[lang]
    const ref = requestIdText(err, lang) ?? (b?.request_id ? fill(reference[lang], { id: b.request_id }) : undefined)
    return ref ? `${base} ${ref}` : base
  }

  if (code && ERROR_CODES[code]) {
    if (code === 'field_too_long') {
      const field = b?.field ?? apiErrorField(err)
      const msg = b?.error ?? apiErrorMessage(err) ?? ''
      const max = /max\.? (\d+)/i.exec(msg)?.[1]
      const name = field ? fieldLabel(field, lang) : lang === 'id' ? 'Isian' : 'A field'
      if (max) return fill(ERROR_CODES.field_too_long[lang], { field: name, max })
      return lang === 'id' ? `${name} terlalu panjang.` : `${name} is too long.`
    }
    if (code === 'rate_limited') {
      const seconds = apiRetryAfter(err)
      return seconds !== undefined ? fill(ERROR_CODES.rate_limited[lang], { seconds: Math.max(1, seconds) }) : BY_STATUS[429][lang]
    }
    if (code === 'export_too_large') return exportTooLargeText(b?.error ?? apiErrorMessage(err), lang)
    if (code === 'delivery_failed') {
      const reason = deliveryReasonText(b?.reason ?? apiErrorReason(err), lang)
      return reason ? `${ERROR_CODES.delivery_failed[lang]} ${reason}` : ERROR_CODES.delivery_failed[lang]
    }
    return ERROR_CODES[code][lang]
  }

  if (isAxios && status === undefined) return network[lang]
  if (status !== undefined && BY_STATUS[status]) {
    // English users still get the server's own words when they are specific.
    const raw = b?.error ?? apiErrorMessage(err)
    if (lang === 'en' && raw && status !== 401 && status !== 403) return raw
    return BY_STATUS[status][lang]
  }
  // Other statuses, or not an HTTP error at all (a thrown Error): never
  // show raw technical text.
  return generic[lang]
}
