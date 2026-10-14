/** Error codes of /push/* (docs/api.md). Merged into `ERROR_CODES`. */
type Pair = { en: string; id: string }

export const PUSH_ERROR_CODES: Record<string, Pair> = {
  push_disabled: {
    en: 'Push notifications are not set up on this server. Ask your administrator.',
    id: 'Notifikasi push belum disiapkan di server ini. Hubungi administrator Anda.',
  },
  too_many_subscriptions: {
    en: 'You already receive notifications on 20 devices. Remove one under "Your devices" first.',
    id: 'Anda sudah menerima notifikasi di 20 perangkat. Hapus salah satu di "Perangkat Anda" terlebih dahulu.',
  },
  invalid_endpoint: {
    en: 'This browser’s push service is not accepted by the server. Try a current Chrome, Edge, Firefox or Safari.',
    id: 'Layanan push browser ini tidak diterima server. Coba Chrome, Edge, Firefox atau Safari versi terbaru.',
  },
  invalid_keys: {
    en: 'The browser sent an invalid notification key. Reload the page and turn notifications on again.',
    id: 'Browser mengirim kunci notifikasi yang tidak valid. Muat ulang halaman lalu aktifkan notifikasi lagi.',
  },
  invalid_event: { en: 'This alert is not available for your role.', id: 'Peringatan ini tidak tersedia untuk peran Anda.' },
}
