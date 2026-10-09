# SecurePatrol web — design & UX guide

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
7. **Accessible:** labelled controls (`Field`), focus-visible rings, dialogs via
   `Modal`, live regions for alerts, colour never the only signal.
8. **Responsive:** works from 360 px; tables scroll horizontally, cards stack.

## Building blocks (src/components/ui)

| Component | Use |
|---|---|
| `PageHeader` | `icon`, `title`, `subtitle` (purpose), `actions` (primary CTA right). |
| `Card` | Sections. `title`, `description`, `icon`, `actions`; `flush` for edge-to-edge lists/tables. |
| `Toolbar` | Filter bar (white card). Put filters + "Export" here. |
| `Table` | Data tables (`columns`, `rows`, loading/error/empty built in). |
| `Tabs` | Pill tabs under the header; keep the tab in `?tab=` (`useTabParam`). |
| `Modal` | Forms (`variant="center"`) and detail panels (`variant="drawer"`). |
| `Button` / `buttonClass()` | `primary`, `secondary`, `soft`, `ghost`, `danger`; sizes `sm`/`md`/`lg`. Use `buttonClass()` from `ui/styles` to style a `<Link>`. |
| `Badge`, `StatusBadge`, `SeverityBadge`, `Dot` | Status pills; `Dot pulse` for "live". |
| `EmptyState`, `ErrorState`, `Notice` | Empty / error / inline info or warning. |
| `Field`, `Input`, `Select`, `Textarea`, `Checkbox`, `FormError` | Forms; always give `hint` for non-obvious fields. |
| `Popover` | Small dropdown menus. |

Hooks: `useSite()` (selected location), `useAuth().can(cap)` (role checks —
hide actions the user cannot perform), `useTabParam`, `useNewParam(open)`
(open the create form when the URL has `?new=1`; the setup guide links there).

## Visual tokens

- Neutral scale: `gray-*` (mapped to slate). Page background `gray-50`, cards white
  with `border-gray-200/80 shadow-card rounded-2xl`.
- Brand: `primary-*` (blue). Shell: `ink-*` (navy).
- Type: Inter. Page title `text-2xl font-semibold tracking-tight`; card title
  15 px semibold; body `text-sm text-gray-600`; meta `text-xs text-gray-500`.
- Numbers: `tabular-nums`.
- Spacing: sections `gap-6`, cards `p-5`, list rows `px-5 py-3`.
- Icons: lucide-react, 16–18 px, inside tinted rounded squares for emphasis.

## Copy

All strings go through i18n (`useI18n().t`). Indonesian is the default and
should read naturally (e.g. "Petugas", "Lokasi", "Tanggapi", "Belum masuk").
Incident categories: `incidentTypeLabel(lang, type)`.

## Demo mode

`npm run dev:demo` runs the app on sample data in the browser (no backend,
no Keycloak): role switcher and "start empty" in the purple bar. Use it to
check every role and the empty states. Mock API: `src/demo/mockApi.ts` — keep
it in sync when the API changes.
