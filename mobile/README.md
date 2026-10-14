# GARDA — tablet app

Flutter 3.24 app for the **shared post tablet** at a site (DECISIONS.md D-001, D-024).

## How it works

1. **Server setup** (first launch): server address + organisation code (Keycloak realm).
   Build-time defaults: `--dart-define=SP_SERVER=https://patrol.example.co.id --dart-define=SP_REALM=securepatrol-client1`.
2. **Device sign-in**: the tablet's Keycloak account (realm role `device`, client
   `securepatrol-tablet`, password grant + offline token). It registers itself and
   waits until an admin approves it on the web (*Settings → Tablets*).
3. **Lock screen**: guards pick their name and enter their PIN. It is checked locally
   against the bcrypt hash synced from `/device/roster` (works offline) and, when
   online, by the server (`POST /device/guard-sessions`, D-035) — the server decides
   when it answers within 3 s; its guard token is stored with every queued action and
   sent as `X-Guard-Token`. Home shows *Verified* / *Offline unlock*. Wrong PINs lock
   the guard out (30 s, then 2/10/30/60 min, persisted). No unlock after 7 days
   without a roster sync. **SOS works on the lock screen without a PIN** (tap your
   name, hold SOS). The tablet locks after the site's idle time (30 min during a
   patrol) and after 30 s in the background (the camera excepted).
4. **Field work**: clock in/out (selfie + GPS), QR patrol with live SLA progress,
   incidents (≤ 5 photos), defects, visitor log, SOS (hold 2 s).
5. **Offline outbox**: every action is stored in SQLite first and sent in order (local
   sequence) with client UUIDs and **server-corrected** event times (offset from
   `server_time` / the `Date` header; idempotent, D-006). Photos upload first with
   deterministic keys, EXIF/GPS stripped on the tablet. SOS jumps the queue and
   retries every 10 s; the guard sees a red banner until the server confirms.
   Failures: network → retry forever; 5xx → parked after 50 attempts / 3 days; 429 →
   `Retry-After`; replays answered "already done" → success; `media_exists` →
   uploaded; device-level 403 → whole queue paused; `guard_token_invalid` → resent
   without the token; anything else → parked and shown (translated) as
   "needs attention". Removing an item needs a supervisor PIN or the device password.
6. **Signed checkpoint QR**: `SP1:<checkpoint uuid>:<sig>` is matched to the route
   locally and the raw text sent as `qr_payload` (the server verifies); legacy codes
   match exactly.

```
lib/
  config/env.dart            server config (+ --dart-define presets)
  core/auth/                 Keycloak device auth, DeviceSession (phases, roster, lock)
  core/api/api_client.dart   /api/v1 client (token + X-Guard-ID)
  core/database/             SQLite cache + outbox
  core/services/             GPS, camera, bcrypt PIN check
  features/                  setup, lock (auth), home, attendance, patrol, incident, sos, sync, device
  features/field_actions.dart  all field actions → outbox
  l10n/                      Bahasa Indonesia + English strings
  theme/, shared/widgets/    design tokens shared with the web app
```

## Develop

```bash
flutter pub get
flutter analyze          # (or: dart analyze)
flutter test             # unit tests (test/)
flutter run --dart-define=SP_SERVER=https://<server> --dart-define=SP_REALM=<realm>
```

**Screenshots without a device** — golden tests render real screens with sample data:

```bash
flutter test screenshots/ --update-goldens   # writes screenshots/out/*.png (git-ignored)
```

## Security notes (audit MOB-*)

- **Local data** — `kv` values and outbox bodies/labels/tokens are AES-256-GCM
  encrypted (`lib/core/storage/field_cipher.dart`, pure Dart); the key lives in
  `flutter_secure_storage` (Android Keystore). A plaintext v1 database is migrated
  on first start. Field encryption was chosen over SQLCipher: no native library to
  build/verify per ABI, fully unit-tested, and the plaintext→encrypted migration is
  testable; the trade-off is that non-sensitive columns (kind, status, times) stay
  readable. No Android backup / device transfer (`allowBackup=false`,
  `data_extraction_rules.xml`), `FLAG_SECURE` on the activity.
- **TLS** — Dart's `HttpClient` ignores Android's network security config and
  user CAs. A private CA is **bundled at build time**:
  `cp infra/nginx/certs/securepatrol-ca.crt mobile/assets/certs/securepatrol-ca.crt`
  (git-ignored; see `assets/certs/README.txt`). `http://` servers are refused unless
  debug build or `--dart-define=SP_ALLOW_HTTP=true`. The Android config trusts system
  CAs only (user CAs in debug builds).
- **Protected actions** — sign-out, server change and outbox discard need a supervisor
  PIN or the device account password; sign-out/server change are refused while items
  are queued.
- **Revoked device** — roster, routes, guard state and tokens are deleted at once;
  unsent items stay.
- **Errors & health** — `FlutterError.onError`, `PlatformDispatcher.onError` and the
  root zone write to a local ring buffer; a beacon posts version, queue depth, failed
  items, last sync, clock offset and recent errors to `POST /device/telemetry` every
  15 min (backend endpoint required).
- **Times** — shown in the site's IANA zone (`timezone` package), stored UTC.

## Release build

- App name **GARDA**; launcher icons and the in-app brand mark are generated from
  `branding/icon.svg` (`branding/generate.sh`, see [branding/README.md](../branding/README.md)).
  The Dart package name (`securepatrol`), method channel `securepatrol/app` and local
  database `securepatrol.db` are internal identifiers and keep the old name.
- Package / bundle id: `id.co.garda.tablet`; version from `pubspec.yaml`
  (`version:`) or `--build-name/--build-number` (CI uses the tag and run number).
- Signing: `android/key.properties` (never committed) with `storeFile` (relative to
  `android/app`), `storePassword`, `keyAlias`, `keyPassword` — the CI release job
  writes it from secrets. **Without it a release build fails**; for a local
  `flutter run --release` only: `SP_ALLOW_DEBUG_SIGNING=true flutter run --release`.

## Tablet provisioning

See [docs/runbook.md](../docs/runbook.md) §3 (incl. kiosk / MDM setup). With a
private-CA server certificate, build the APK with the CA bundled (above);
installing the CA in Android settings is not enough for the app.
