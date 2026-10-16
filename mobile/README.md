# GARDA — tablet app

Flutter 3.47 app for the **shared post tablet** at a site (DECISIONS.md D-001, D-024).

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
7. **Push (optional)**: with Firebase configured (below) the tablet reacts at once to
   an SOS from another post, approval/revocation and roster changes; polling keeps
   running as the fallback.

```
lib/
  config/env.dart            server config (+ --dart-define presets)
  core/auth/                 Keycloak device auth, DeviceSession (phases, roster, lock)
  core/api/api_client.dart   /api/v1 client (token + X-Guard-ID)
  core/database/             SQLite cache + outbox
  core/services/             GPS, camera, bcrypt PIN check, push (FCM)
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

## Push notifications (Firebase Cloud Messaging)

Push is **optional**. A build without the values below runs exactly as before: no
Firebase, SOS polled every 15 s (30 s on the lock screen), pending tablets every 10 s.

**Build with push** — four `--dart-define`s (CI: repository secrets of the same names,
used by the release job when all four are set):

```bash
flutter build apk --release   --dart-define=SP_FCM_API_KEY=<API key>   --dart-define=SP_FCM_APP_ID=<1:1234567890:android:abc123...>   --dart-define=SP_FCM_PROJECT_ID=<project id>   --dart-define=SP_FCM_SENDER_ID=<project number>
# iOS (not used for the post tablets yet): also --dart-define=SP_FCM_IOS_APP_ID=...
```

**Firebase project (once per installation or vendor):**
1. [Firebase console](https://console.firebase.google.com) → *Add project* (Google
   Analytics not needed).
2. *Add app → Android* with package name **`id.co.garda.tablet`** (must match the app
   id of the build). No SHA-1 needed for messaging.
3. Read the four values from *Project settings → General*: **Project ID**, **Project
   number** (= sender ID), the Android app's **App ID**, and the **Web API key** (the
   same key is `current_key` in the `google-services.json` the console offers — you
   may read it from there, but don't commit that file).
4. For the server: *Project settings → Service accounts → Generate new private key*;
   the backend sends the messages (see [runbook: Push notifications](../docs/runbook.md)).

**Why no `google-services.json`:** the values come from the build, so one source tree
builds with or without push, nothing Firebase-specific is committed, and the Google
Services Gradle plugin is not needed. `lib/core/services/push_service.dart` calls
`Firebase.initializeApp(options: ...)`; `android/app/build.gradle` decodes the same
`--dart-define`s and writes them as the string resources that plugin would generate
(`google_app_id`, `google_api_key`, `gcm_defaultSenderId`, `project_id`), so Firebase
also initialises when a push starts the app process.

**Behaviour** (`PushService`): the token is sent with `PUT /device/push-token` at
start, on token refresh and after every (re)registration/approval (retried with
backoff), and removed with `DELETE /device/push-token` on device sign-out. Data
messages: `sos` → SOS list refreshed + high-importance notification (channel *SOS*:
sound, vibration; tap → SOS alerts when unlocked, the lock screen shows the SOS strip),
`sos_ack` → refresh + that notification cancelled, `patrol_not_started` → guard
status refreshed + notification (channel *Patroli*; tap → home), `device_status` →
device re-read (approval/revocation/move at once), `sync` → roster and routes; unknown
types are ignored. In the background a handler shows the notification and leaves a
refresh flag the app applies when it comes back. The notification permission
(Android 13+) is asked right after the device registers (again after approval if
still undecided) — never on the lock screen; *About this tablet* shows the status
(*Active / Not configured / Permission denied / Token not sent yet*) with *Ask for
permission again*. While push is **active** (configured + permission + token
registered) SOS polling slows to every 60 s — never slower; otherwise 15 s / 30 s as
before. The health beacon reports `push` (`active` / `unconfigured` / `denied` /
`error`).

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
