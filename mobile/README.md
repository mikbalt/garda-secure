# SecurePatrol — tablet app

Flutter 3.24 app for the **shared post tablet** at a site (DECISIONS.md D-001, D-024).

## How it works

1. **Server setup** (first launch): server address + organisation code (Keycloak realm).
   Build-time defaults: `--dart-define=SP_SERVER=https://patrol.example.co.id --dart-define=SP_REALM=securepatrol-client1`.
2. **Device sign-in**: the tablet's Keycloak account (realm role `device`, client
   `securepatrol-tablet`, password grant + offline token). It registers itself and
   waits until an admin approves it on the web (*Settings → Tablets*).
3. **Lock screen**: guards pick their name and enter their PIN, verified locally
   against the bcrypt hash synced from `/device/roster` — works offline. The tablet
   locks after the site's idle time (relaxed to 30 min during a patrol).
4. **Field work**: clock in/out (selfie + GPS), QR patrol with live SLA progress,
   incidents (≤ 5 photos), defects, visitor log, SOS (hold 2 s).
5. **Offline outbox**: every action is stored in SQLite first and sent in order with
   client UUIDs and event times (idempotent, D-006). Photos upload first with
   deterministic keys. SOS jumps the queue and retries every 10 s; the guard sees a
   red banner until the server confirms.

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

## Tablet provisioning

See [docs/runbook.md](../docs/runbook.md) §3. With a self-signed server certificate,
install the certificate on the tablet (Settings → Security → Install certificate);
the app trusts user-installed CAs (`android/app/src/main/res/xml/network_security_config.xml`).
