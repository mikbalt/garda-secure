// Strings of the push notifications (Firebase Cloud Messaging). Merged into [S].

const pushStringsId = <String, String>{
  'push.title': 'Notifikasi push',
  'push.active': 'Aktif',
  'push.unconfigured': 'Tidak dikonfigurasi',
  'push.denied': 'Izin ditolak',
  'push.tokenPending': 'Token belum terkirim',
  'push.error': 'Gangguan',
  'push.hintActive': 'SOS dan perubahan status tablet langsung masuk. Tablet tetap memeriksa SOS setiap menit.',
  'push.hintFallback': 'Tanpa push, tablet memeriksa SOS setiap 15 detik (30 detik saat terkunci).',
  'push.askAgain': 'Minta izin lagi',
  'push.retry': 'Kirim ulang token',
  'push.deniedSettings': 'Izin notifikasi ditolak. Aktifkan notifikasi GARDA di Pengaturan Android.',
  // Android notification channels (names are fixed: SOS, Patroli, Sistem)
  'push.channelSos': 'Peringatan SOS dari pos lain di lokasi ini',
  'push.channelPatrol': 'Patroli terjadwal yang belum dimulai',
  'push.channelSystem': 'Status tablet dan sinkronisasi',
  'push.patrolTitle': 'Patroli belum dimulai',
};

const pushStringsEn = <String, String>{
  'push.title': 'Push notifications',
  'push.active': 'Active',
  'push.unconfigured': 'Not configured',
  'push.denied': 'Permission denied',
  'push.tokenPending': 'Token not sent yet',
  'push.error': 'Error',
  'push.hintActive': 'SOS and tablet status changes arrive at once. The tablet still checks for SOS every minute.',
  'push.hintFallback': 'Without push the tablet checks for SOS every 15 seconds (30 seconds when locked).',
  'push.askAgain': 'Ask for permission again',
  'push.retry': 'Send token again',
  'push.deniedSettings': 'Notification permission denied. Turn on GARDA notifications in Android Settings.',
  'push.channelSos': 'SOS alerts from other posts at this site',
  'push.channelPatrol': 'Scheduled patrols not started',
  'push.channelSystem': 'Tablet status and sync',
  'push.patrolTitle': 'Patrol not started',
};
