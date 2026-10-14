#!/usr/bin/env bash
# Regenerates every GARDA icon from branding/icon.svg (see branding/README.md).
#
#   branding/generate.sh             web + tablet icons
#   branding/generate.sh --web-only  skip the Flutter launcher icons
#
# Needs: Node + `npm ci` in web/ (Playwright renders the SVG; set
# PW_CHANNEL=chrome to use the installed Google Chrome instead of Playwright's
# Chromium) and, for the tablet icons, Flutter on PATH.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
root="$(dirname "$here")"

echo "== rendering PNG/ICO from branding/icon.svg"
(cd "$root/web" && node scripts/brand-icons.mjs)

if [[ "${1:-}" == "--web-only" ]]; then
  echo "== skipped Flutter launcher icons (--web-only)"
  exit 0
fi

echo "== Flutter launcher icons (Android mipmaps + adaptive icon, iOS AppIcon)"
(cd "$root/mobile" && dart run flutter_launcher_icons)

echo "== done. Review with: git status branding web/public mobile"
