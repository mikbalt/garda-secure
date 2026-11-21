# Reports & exports

Staff and clients read reports for a site and download them: patrol-schedule compliance, the monthly attendance recap for payroll, and incident summaries.

## Sub-features

- `reports-index` lists the reports the role may open.
- `reports-compliance` shows how many scheduled patrols were done on time.
- `reports-export-xlsx` downloads the open report as an Excel file.
- `reports-monthly` shows the monthly recap in guard-days with a site total.
- `reports-incidents` summarises incidents.

## How to get to it (user POV)

- Choose `Laporan` in the main navigation (`/reports`), then a report link such as `Kepatuhan jadwal patroli`.
- Open a report directly: `/reports?tab=compliance`, `/reports?tab=monthly`, `/reports?tab=incidents`.
- In a report, choose `Unduh`, then a format.

## Driving it with verify.mjs

Preconditions:

- `DOCTOR OK` on the instance.
- The role is `supervisor`.

Run `node .claude/skills/verify/verify.mjs drive .claude/skills/verify/journeys/reports-compliance-export.mjs`.

- **Open the index.** `openAs('supervisor', '/reports')`. The link `Kepatuhan jadwal patroli` shows.
- **Open the report.** `goto('/reports?tab=compliance')`. The heading and the region `Kepatuhan jadwal patroli` show.
- **Download.** Click `Unduh`, then `Excel (.xlsx)` inside `download(...)`. A file ending in `.xlsx` is saved into the evidence directory.
- **Proof.** The saved file is not empty and starts with the zip signature `PK`. Screenshots `01` and `02` show the index and the report.

## Gotchas

- A download event alone is not proof. Save the file and check it, as the journey does.
- The file name carries a date from the report period, so match the extension, not the name.
- The demo builds the file in the browser. In the real stack the API builds it. Check the content against the API there.
- Report access differs per role (`web/src/lib/roles.ts`): `hr` gets the attendance reports but not the patrol and incident ones. Prove a role's set as that role.
- Not driven: `reports-monthly`, `reports-incidents`, the PDF format, and the index link as the way into the report (the journey uses the URL). See `web/e2e/patrol.spec.ts`.
