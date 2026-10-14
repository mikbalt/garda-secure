# GARDA branding — app icon

Product name: **GARDA** (all caps, no tagline). Every icon of the web app and
the tablet app is generated from **one file**: [`icon.svg`](icon.svg).
The current icon is a placeholder (shield + check on the brand blue).

## Replace the icon

1. Replace `branding/icon.svg` with the new artwork:
   - square (any viewBox, e.g. `0 0 1024 1024`), **full-bleed and opaque**
     — do not round the corners, the platforms do that;
   - keep the logo inside the **central 80% circle** (Android/PWA masks cut the rest);
   - optional but recommended: give the background element `id="background"`
     and the logo group `id="mark"`. The generator uses them to build the
     Android adaptive icon layers (logo on a separate layer). Without the ids
     the whole icon is used as the foreground layer, which also works.
   - SVG only, no external references (fonts as paths, images embedded).
2. Regenerate (from the repo root):

   ```bash
   (cd web && npm ci)                  # once, installs Playwright
   PW_CHANNEL=chrome branding/generate.sh   # or without PW_CHANNEL if Playwright's Chromium is installed
   ```

   No Flutter on the machine? `branding/generate.sh --web-only`, then run
   `cd mobile && dart run flutter_launcher_icons` on a machine with Flutter.
3. Check the result (`git status`), open `branding/icon-1024.png`, run the app
   (`cd web && npm run dev:demo`) and commit everything that changed.

## What gets generated

| Output | Used by |
| --- | --- |
| `branding/icon-1024.png` | master PNG; source of the Flutter launcher icons (iOS App Store icon) |
| `branding/generated/android-foreground.png`, `android-background.png` | Android adaptive icon layers |
| `web/public/favicon.svg` (copy of icon.svg) | browser favicon **and** the in-app `<BrandMark>` |
| `web/public/favicon.ico` (16/32/48) | legacy favicon |
| `web/public/apple-touch-icon.png` (180) | iOS/iPadOS home screen |
| `web/public/icons/icon-192.png`, `icon-512.png`, `maskable-512.png`, `favicon-32.png` | PWA manifest (`web/public/manifest.webmanifest`) |
| `mobile/assets/brand/icon.png` (256) | tablet app in-app brand mark (`shared/widgets/brand.dart`) |
| `mobile/android/app/src/main/res/mipmap-*`, `drawable-*`, `mipmap-anydpi-v26/` | Android launcher icon (via `flutter_launcher_icons`) |
| `mobile/ios/Runner/Assets.xcassets/AppIcon.appiconset/` | iOS app icon (via `flutter_launcher_icons`) |

Do not edit the generated files by hand. Colours that are not in the SVG:
`theme_color` / `background_color` in `web/public/manifest.webmanifest` and
`<meta name="theme-color">` in `web/index.html` (brand blue `#1f38d4`, shell navy `#0d1420`).

## How it works

- `web/scripts/brand-icons.mjs` (`npm run icons` in `web/`) renders the SVG
  with Playwright's headless Chromium into all PNG sizes and the ICO.
- `mobile/pubspec.yaml` → `flutter_launcher_icons:` turns `icon-1024.png` and
  the two adaptive layers into the Android and iOS icon sets.
- `branding/generate.sh` runs both.

App names that are **not** icons: web `app.name` (i18n), `<title>` in
`web/index.html`, the manifest `name`, Android `android:label`
(`AndroidManifest.xml`), iOS `CFBundleDisplayName` (`Info.plist`), tablet
`app.name` (`mobile/lib/l10n/strings.dart`), Keycloak realm `displayName`
(`infra/keycloak`). Technical identifiers (Go module, binary `securepatrol`,
Keycloak client IDs, database names) intentionally keep the old name.
