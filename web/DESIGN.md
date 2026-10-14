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

- Neutral scale: `gray-*` (mapped to slate). Page background `gray-50`, cards white
  with `border-gray-200/80 shadow-card rounded-2xl`.
- **Text contrast on white/gray-50:** body `text-gray-600`+ , meta/hints
  `text-gray-500` (4.8:1) at least. `text-gray-400`/`300` only for decorative
  icons (`aria-hidden`), disabled controls, or on the dark shell (`ink-*`),
  where `text-gray-300/400` are the readable greys. Coloured text on tinted
  backgrounds uses the `-700`/`-800` shade (e.g. `text-amber-800` on
  `bg-amber-50`).
- Brand: product name **GARDA** (all caps, `app.name`; no tagline). Primary `primary-*` (blue). Shell: `ink-*` (navy).
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
