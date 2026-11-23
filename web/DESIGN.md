# GARDA web — design & UX guide

Read this before changing any page. The goal: an operations console that a
security company's admin, supervisor, HR officer or client understands
**without training** — every screen says what it is for, what needs attention,
and what to do next.

## Principles

1. **Lead with what needs action.** Exceptions (SOS, overdue checkpoints,
   absences, items to review) come before totals and charts.
2. **One clear primary action per screen**, top-right in the page header
   (e.g. "Tambah petugas"). Secondary actions are `secondary`/`ghost` buttons.
3. **Explain the screen in one sentence** (`PageHeader` `subtitle`). Write for
   a non-technical user in plain Indonesian; English is the second language.
4. **Empty states teach.** Never just "Tidak ada data": say why it is empty and
   offer the action that fills it (`EmptyState` with `title`, `message`, CTA).
5. **Fewer clicks for daily tasks.** Date steppers (◀ Hari ini ▶) instead of
   bare date inputs, clickable status summary chips that filter tables,
   drawers for details instead of separate pages.
6. **Consistent status language:** `StatusBadge` / `SeverityBadge` (dot +
   colour). Green = ok, amber = late/warning, orange = overdue/open, red =
   absent/critical/failed, blue = in progress, grey = pending.
7. **Accessible (WCAG 2.1 AA, checked by axe in the e2e smoke run):** labelled
   controls (`Field` wires hint/error via `aria-describedby`), focus-visible
   rings, dialogs via `Modal`, colour never the only signal. Live regions
   (`role="alert"`/`status`) carry **text only** — buttons go outside them.
   Wide, horizontally scrolling tables are focusable regions (`Table` does it).
8. **Responsive:** works from 360 px; tables scroll horizontally, cards stack.

## Building blocks (src/components/ui)

| Component | Use |
|---|---|
| `PageHeader` | `icon`, `title`, `subtitle` (purpose), `actions` (primary CTA right). |
| `Card` | Sections. `title`, `description`, `icon`, `actions`; `flush` for edge-to-edge lists/tables. |
| `Toolbar` | Filter bar (white card). Put filters + "Export" here. |
| `Table` | Data tables (`columns`, `rows`, loading/error/empty built in). Give a row-action column `stickyEnd` so it stays pinned (visible) while a wide table scrolls. |
| `Tabs` + `TabPanel` | Pill tabs under the header; keep the tab in `?tab=` (`useTabParam`). Give both the same `idBase`; wrap the active content in `TabPanel` (or `MaybeTabPanel` when the tab list is conditional). |
| `Modal` | Forms (`variant="center"`) and detail panels (`variant="drawer"`). |
| `Button` / `buttonClass()` | `primary`, `secondary`, `soft`, `ghost`, `danger`; sizes `sm`/`md`/`lg`. Use `buttonClass()` from `ui/styles` to style a `<Link>`. |
| `Badge`, `StatusBadge`, `SeverityBadge`, `Dot` | Status pills; `Dot pulse` for "live". |
| `EmptyState`, `ErrorState`, `Notice` | Empty / error / inline info or warning. |
| `Field`, `Input`, `Select`, `Textarea`, `Checkbox`, `FormError` | Forms; always give `hint` for non-obvious fields. |
| `Popover` | Small dropdown panels — WAI-ARIA *disclosure* (trigger with `aria-expanded`, ordinary buttons/links inside, Escape returns focus). Do not add `menu`/`listbox` roles unless you implement the full arrow-key pattern. |
| `FlagBadges` / `FlagIndicator` | Plausibility flags of a record (`flags`, `scan_flags`): amber badges whose disclosure explains the flag (EN/ID, unknown flags generic). Use `FlagIndicator` (no button) inside rows that are themselves buttons. |
| `SecretOnce` | A secret the API returns once (temporary / tablet password): "shown only once" warning, read-only field, copy button. Never store it. |

Hooks: `useSite()` (selected location), `useAuth().can(cap)` (role checks —
hide actions the user cannot perform; `canSeeMedia(kind)` for photos),
`useTabParam`, `useNewParam(open)` (open the create form when the URL has
`?new=1`; the setup guide links there), `useActiveSos()` (open SOS alerts of
all visible sites), `usePush()` (web push state of this browser —
`unsupported` / `ios-needs-install` / `server-disabled` / `blocked` / `off` /
`on` — with enable/disable; logic in `lib/push.ts`).

Push notifications: `/notifications` (user menu, every web role) is a
personal page, **not** site-scoped (no `RequireSite`). The service worker
`public/sw.js` only shows pushes and handles clicks (no fetch handler, no
caching); a click posts `garda:navigate` to an open window
(`useServiceWorkerNavigation` in the Layout) or opens one. Admins and
supervisors get a one-time dashboard hint (`PushNudge`) while push is `off`.

Roles: `platform-admin` (install-wide operator) sees Settings → Channels, Ops
alerts, Ops log, Backups; a realm `admin` without it sees an explanation
there instead. The app shell has one `ErrorBoundary` per route (friendly text
+ request id, never raw error text), the realm-wide SOS banner and the idle
logout dialog.

## Visual tokens

Direction: **Buku Jaga** (the guard's logbook, TAQ-42): warm paper, ink-black
actions, mono for evidence, **no gradients**, muted status colours. Tokens are
CSS variables ("R G B" channels, so `/80` opacity works) in `src/index.css`:
`:root` is light, `.dark` on `<html>` is dark (no toggle yet). Tailwind reads
them in `tailwind.config.ts`; `src/lib/tokens.test.ts` enforces the table below.

- Neutral scale: `gray-*` (warm paper to ink; flips in dark). Page background
  `gray-50` (paper), cards `bg-white` + `border-gray-200` + `shadow-card` (none:
  flat, 1px border; shadows only for `raised`/`overlay`). Semantic names also
  exist: `paper`, `surface`, `action-fg`, and status pairs `text-ok bg-ok-bg`
  (`ok`, `warn`, `overdue`, `danger`) plus `bg-sos text-sos-fg`. Keep dot + word:
  colour is never the only signal.
- Action: `primary-*` is the ink scale; `primary-600` is the button (white text in
  light; in dark it becomes the paper button, so use `text-action-fg`, not
  `text-white`, on new buttons). Focus ring: 2px ink + 2px offset (global
  `:focus-visible`). Shell: `ink-*` (always dark).
- Radius: 6 px (`rounded-md`/`lg`: inputs, chips), 10 px (`rounded-xl`/`2xl`: cards).
- **Text contrast on the page:** body `text-gray-600`+, meta/hints `text-gray-500`
  (5.7:1) at least. `text-gray-400`/`300` only for decorative icons
  (`aria-hidden`), disabled controls, or on the dark shell (`ink-*`). Coloured
  text on tinted backgrounds uses the status pair, not a raw palette colour.

| Pair (fg on bg) | Light | Ratio | Dark | Ratio |
|---|---|---|---|---|
| `text` on `paper` | #15181E on #F5F2EA | 15.9 | #ECE9E1 on #0E1116 | 15.6 |
| `text` on `surface` | #15181E on #FFFDF8 | 17.5 | #ECE9E1 on #161A21 | 14.4 |
| `muted` on `paper` (= `gray-500` on `gray-50`) | #5B6068 on #F5F2EA | 5.7 | #9AA0AA on #0E1116 | 7.2 |
| `muted` on `surface` | #5B6068 on #FFFDF8 | 6.2 | #9AA0AA on #161A21 | 6.6 |
| `action-fg` on `action` | #FFFFFF on #1B2A4A | 14.2 | #0E1116 on #ECE9E1 | 15.6 |
| `ok` on `ok-bg` | #1E6B4A on #E4F0E8 | 5.5 | #5FC59A on #14281F | 7.3 |
| `warn` on `warn-bg` | #7A4F00 on #F6E7C1 | 5.8 | #E2B24A on #2E2410 | 7.8 |
| `overdue` on `overdue-bg` | #9A3D0C on #F9E4D6 | 5.6 | #F08A4B on #301D10 | 6.4 |
| `danger` on `danger-bg` | #A4191F on #F8DDDC | 6.0 | #FF7B7B on #321517 | 6.7 |
| `sos-fg` on `sos` | #FFFFFF on #C01B2C | 6.1 | #FFFFFF on #D6293B | 5.0 |
| `border-strong` on `paper` (non-text, 3:1) | #8F8A7C on #F5F2EA | 3.1 | #7C828D on #0E1116 | 4.9 |
| `focus` on `paper` (non-text, 3:1) | #1B2A4A on #F5F2EA | 12.7 | #ECE9E1 on #0E1116 | 15.6 |

Known gaps for later revamp tasks: `bg-white` and `text-white` are not themed
yet (dark mode needs components moved to `surface`/`action-fg`); fonts
(IBM Plex) and the brand mark are separate tasks.
- Brand: product name **GARDA** (all caps, `app.name`; no tagline).
- App mark: `<BrandMark>` (`components/BrandMark.tsx`) renders `public/favicon.svg`, a copy of
  `branding/icon.svg` — never draw the logo by hand; replace the icon via `branding/generate.sh`.
- Type: Inter. Page title `text-2xl font-semibold tracking-tight`; card title
  15 px semibold; body `text-sm text-gray-600`; meta `text-xs text-gray-500` (never lighter).
- Numbers: `tabular-nums`.
- Spacing: sections `gap-6`, cards `p-5`, list rows `px-5 py-3`.
- Icons: lucide-react, 16–18 px, inside tinted rounded squares for emphasis.

## Copy

All strings go through i18n (`useI18n().t`). Indonesian is the default and
should read naturally (e.g. "Petugas", "Lokasi", "Tanggapi", "Belum masuk").
Incident categories: `incidentTypeLabel(lang, type)`.

## Demo mode

`npm run dev:demo` runs the app on sample data in the browser (no backend,
no Keycloak): role switcher (incl. *Platform operator*) and "start empty" in
the purple bar. Use it to check every role and the empty states. Mock API:
`src/demo/mockApi.ts` — keep it in sync when the API changes (it also enforces
role checks, `secret_required`, `media_forbidden` and records audit rows).
A container with `DEMO=true` refuses demo mode when its runtime config points
at a real server (absolute `API_URL`/`KEYCLOAK_URL`) unless `DEMO_FORCE=true`.

## Tests

- `npm test` — Vitest + Testing Library (roles matrix, site-TZ maths, error
  i18n, EN/ID parity, API client, components).
- `npm run e2e` — Playwright against the demo build served with the
  production CSP: every role × key page at 1280/390 px without console or CSP
  errors and without serious/critical axe violations, keyboard-only paths and
  feature checks. Locally `PW_CHANNEL=chrome npm run e2e` uses the installed
  Chrome. Add a page to `e2e/smoke.spec.ts` when you add a route.
