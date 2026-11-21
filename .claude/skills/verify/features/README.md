# GARDA verification map

This directory is the maintained source for verifying the user-facing behaviour of the GARDA web dashboard. Read this index before driving the app, then use the matching feature file as the recipe.

## Baseline preconditions

- Launch with `node .claude/skills/verify/verify.mjs launch` and require `READY http://127.0.0.1:4273`.
- Run `node .claude/skills/verify/verify.mjs doctor` and require `DOCTOR OK`.
- Every drive starts from the sample data: sites `Gudang Cikarang` and `Kantor Pusat Sudirman`, admin `Rina Kusuma`, two open SOS alerts.
- Never drive an instance that this skill did not launch.

## Driving conventions

- Run every journey with `node .claude/skills/verify/verify.mjs drive .claude/skills/verify/journeys/<name>.mjs`.
- Each drive opens a fresh browser, so data written by one drive is gone in the next. Steps that depend on each other belong in one journey.
- Use ARIA roles and the Indonesian accessible names, not CSS selectors or position.
- A recipe's entry points are all part of the feature. An entry point marked `not driven` under `Gotchas` has no journey yet.

## Proof and skip reporting

- Capture the action and the resulting state: a `shot` before, a `shot` after, and the stored value (`world()`, a reopened dialog or a downloaded file).
- Record the feature file and the entry point used with the evidence directory.
- These recipes prove the web app on demo data. They do not prove the Go API, SQL, Keycloak or tenancy. Report that limit with every proof.
- Report an unreachable path with the command you ran and the precondition that was not met.
- Do not report an entry point that was skipped as verified through a different one.

## Feature entry contract

Each feature file starts with an H1 title and one paragraph that describes the user-visible behaviour. It then uses exactly four H2 sections in this order.

1. `Sub-features` lists short IDs with one line for each behaviour.
2. `How to get to it (user POV)` lists every user entry point.
3. `Driving it with verify.mjs` starts with `Preconditions:` and pairs each user action with the journey step and the observable result.
4. `Gotchas` lists traps that can waste or invalidate a verification run.

Keep implementation details out of the map.

## Features

| Feature | File | Journey | Roles |
|---|---|---|---|
| Users & access | [users-access.md](users-access.md) | `users-create.mjs` | admin |
| SOS alerts | [sos-alerts.md](sos-alerts.md) | `sos-acknowledge.mjs` | admin, supervisor |
| Patrol routes & schedules | [patrol-routes.md](patrol-routes.md) | `patrol-route-schedule.mjs` | admin |
| Attendance & corrections | [attendance.md](attendance.md) | `attendance-history.mjs` | admin, supervisor, hr, client-readonly |
| Reports & exports | [reports.md](reports.md) | `reports-compliance-export.mjs` | admin, supervisor, hr, client-readonly |

Not mapped yet: dashboard and setup guide, guards and PINs, shift schedules, incidents and visitors, settings (rules, devices, channels, backups), audit log, push notifications, the Flutter tablet app. `web/e2e/*.spec.ts` covers most of them and is the place to copy selectors from.
