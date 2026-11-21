# Attendance & corrections

Staff see who clocked in and out at a site on a given day, review flagged records, correct a record with a reason, and read the history of those corrections.

## Sub-features

- `attendance-daily` lists the day's records for the selected site.
- `attendance-history` shows every correction of a record, oldest first, in the site's time zone.
- `attendance-correct` changes a record and requires a reason.
- `attendance-review` lists flagged records with an explanation of each flag.

## How to get to it (user POV)

- Choose `Kehadiran` in the main navigation (`/attendance`).
- Pick a day with the date control, or open `/attendance?date=YYYY-MM-DD`.
- Choose `Riwayat — <guard>` on a corrected record.
- Open the review queue at `/attendance?tab=review`.

## Driving it with verify.mjs

Preconditions:

- `DOCTOR OK` on the instance.
- The sample data has a record from yesterday (Asia/Jakarta) with two corrections by `admin.rina`.

Run `node .claude/skills/verify/verify.mjs drive .claude/skills/verify/journeys/attendance-history.mjs`.

- **Open yesterday.** `openAs('hr', '/attendance?date=<yesterday in Asia/Jakarta>')`. The day's records show.
- **Open the history.** Click the first `Riwayat — ...` button. The dialog `Riwayat koreksi — <guard>` appears.
- **Proof.** The dialog shows `2 koreksi, terlama lebih dulu. Waktu menurut zona waktu lokasi.`, the reasons containing `mengantar korban kecelakaan` and `tablet mati`, and the user `admin.rina`. Screenshots `01` and `02`.

## Gotchas

- "Yesterday" is the site's day, not the machine's. Compute it in Asia/Jakarta, as the journey does. A run just after midnight UTC breaks a naive date.
- Sample records are generated relative to today, so a fixed date in a journey goes stale.
- `client-readonly` sees attendance but gets no selfie buttons and no correction button. Prove that as that role.
- Not driven: `attendance-correct`, `attendance-review`, the date control entry point. See `web/e2e/layout.spec.ts` and `web/e2e/features.spec.ts`.
- Day boundaries are computed by the API in the site's time zone. The demo proves the display, not that computation.
