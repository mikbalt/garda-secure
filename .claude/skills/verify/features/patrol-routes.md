# Patrol routes & schedules

An administrator defines patrol routes: the checkpoints, where they sit on the floor plan, when patrols are due, and the QR codes guards scan at each checkpoint.

## Sub-features

- `route-schedule` sets the interval, time window and grace period, with a summary of patrols per night.
- `route-grace-rule` rejects a grace period longer than half the interval.
- `route-plan-marker` places a checkpoint on the floor plan with mouse or arrow keys.
- `route-qr-regenerate` regenerates signed QR codes after a confirmation and opens the print sheet.
- `route-live-plan` shows the running patrol on the floor plan.

## How to get to it (user POV)

- Choose `Patroli` in the main navigation, then the tab for routes (`/patrol?tab=routes`).
- Choose `Ubah — <route>` on a route to open the dialog `Ubah rute`, with tabs for checkpoints, `Denah` and `Jadwal`.
- Choose `Cetak kode QR — <route>` for the QR sheet.
- Open `/patrol?view=plan` for the live floor plan.

## Driving it with verify.mjs

Preconditions:

- `DOCTOR OK` on the instance.
- The role is `admin`. The route `Patroli Keliling Gudang` exists with a schedule of every 2 hours from 22:00 to 06:00 and a grace of 15 minutes.

Run `node .claude/skills/verify/verify.mjs drive .claude/skills/verify/journeys/patrol-route-schedule.mjs`.

- **Open the editor.** `openAs('admin', '/patrol?tab=routes')`, click `Ubah — Patroli Keliling Gudang`. The dialog `Ubah rute` appears.
- **Read the schedule.** Click the tab `Jadwal`. The summary starts with `Setiap 2 jam dari 22:00 sampai 06:00`.
- **Break the rule.** Fill the first `Toleransi` field with `90`. The message `Toleransi maksimal setengah dari selang waktu.` appears.
- **Save a valid value.** Fill `20`, click `Simpan`. The text `Rute tersimpan.` appears.
- **Proof.** Reopen `Ubah — Patroli Keliling Gudang`, tab `Jadwal`: the field `Toleransi` holds `20`. Screenshots `01` to `04` show each state.

## Gotchas

- A route can have several schedules, so `Toleransi` matches several fields. Use `.first()` or scope to one schedule.
- The save toast alone is not proof. Reopen the editor.
- Checkpoint markers are buttons named `Titik <n>, <name>, di X <n> %`; their name changes when they move.
- Not driven: `route-plan-marker`, `route-qr-regenerate`, `route-live-plan`. `web/e2e/patrol.spec.ts` has working selectors.
- Whether a scan on a tablet matches a schedule is decided by the API. The demo does not prove it.
