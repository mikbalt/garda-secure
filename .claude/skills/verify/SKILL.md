---
name: verify
description: Drive the GARDA (garda-secure) web dashboard the way a user does and capture proof - launches the demo build under the production CSP, runs a scripted browser journey as a chosen role, and saves screenshots, a trace, downloads and the resulting data. Use to prove a web/UI change works, to reproduce a UI bug, or whenever a task says "verify", "prove it works" or "show it working". Also says how to reach the real Docker stack and the backend integration tests when the change is not in the web app.
---

# Verify GARDA

GARDA has three surfaces. Pick the tier that matches what you changed.

| You changed | Tier | Proves | Needs |
|---|---|---|---|
| `web/` (pages, components, i18n, demo mock) | **1. Web demo** (this skill's helper) | UI behaviour, roles, i18n, CSP, downloads | Node + `web/node_modules` |
| `backend/` (handlers, services, SQL, migrations) | **2. Backend integration tests** | API behaviour against a real Postgres 16 | Go 1.26 |
| Anything that crosses web <-> API <-> Keycloak | **3. Real stack** | The whole path with real auth and data | Docker |
| `mobile/` | `dart analyze && flutter test` in `mobile/` | Unit/widget level only | Flutter |

**Tier 1 runs on sample data in the browser (`web/src/demo/mockApi.ts`). It proves
the web app; it proves nothing about the Go API, SQL, Keycloak or tenancy.** If
your change touches an API contract, tier 1 only shows that the UI and the mock
agree. Say which tier your proof came from.

All commands run from the repository root. Evidence lands in
`.claude/verify/evidence/` (git-ignored) and survives cleanup.

## Tier 1: web demo

The helper is `.claude/skills/verify/verify.mjs`. It needs `web/node_modules`
(`npm ci` in `web/` once) and a browser: Playwright's Chromium
(`npx playwright install chromium` in `web/`) or an installed Google Chrome.
It tries Chromium first and falls back to Chrome; set `PW_CHANNEL=chrome` to
skip the first attempt.

### Launch

```bash
node .claude/skills/verify/verify.mjs launch            # port 4273
node .claude/skills/verify/verify.mjs launch --port 4274  # a second, independent instance
```

Builds the demo bundle (`vite build --mode demo`) into
`.claude/verify/instance-<port>/dist` and serves it with `vite preview`, which
adds the production Content-Security-Policy. It prints `READY http://127.0.0.1:<port> (pid N)`
only after the app shell, its entry script and the CSP header are all served.
There is no database, migration or seed step: the sample data is created in the
browser on first load.

The build does not run `tsc`. Type errors are not a launch failure; run
`npx tsc --noEmit` in `web/` separately.

Isolation: each port has its own build directory and process, so instances do
not disturb each other, `web/dist`, or the e2e suite's server on port 4173.
Launch refuses a port that something else already holds.

Every command takes `--port` (or `VERIFY_PORT`). Use the same port for launch,
doctor, drive and cleanup.

### Doctor

```bash
node .claude/skills/verify/verify.mjs doctor
```

Read-only. Run it first whenever something looks off. It checks that the
instance was started by this skill, the process is alive, the page, entry script
and CSP header are served, the build is from the current commit, and no file
under `web/` changed since the build. `DOCTOR FAILED` on the last two means you
are looking at old code: run cleanup, then launch. `drive` runs the same checks
and refuses a failing instance.

### Drive

```bash
node .claude/skills/verify/verify.mjs drive .claude/skills/verify/journeys/users-create.mjs
node .claude/skills/verify/verify.mjs drive <journey.mjs> --name my-proof --mobile   # 390x844, touch
node .claude/skills/verify/verify.mjs drive <journey.mjs> --headed                   # watch it
```

A journey is one `.mjs` file with a default export. It gets a Playwright `page`
plus helpers; it imports nothing from Playwright itself.

```js
export default async function ({ page, expect, openAs, goto, shot, note, world, download, settle, quiet, evidenceDir }) {
  await openAs('admin', '/users')                 // role, path, optional 'id' | 'en'
  await shot('users list before')                 // numbered screenshot
  await page.getByRole('button', { name: 'Tambah pengguna' }).first().click()
  // ...
  expect(JSON.stringify(await world())).toContain('spv.sari')   // the data behind the UI
  note('what this run observed')
}
```

| Helper | Does |
|---|---|
| `openAs(role, path = '/', lang = 'id')` | Opens the app as `admin`, `supervisor`, `hr`, `client-readonly` or `platform-admin`, mutes the SOS alarm, waits for the page to settle. |
| `goto(path)` | Navigates in the same tab and settles. Data written earlier in the journey is still there. |
| `shot(label)` | Saves `NN-label.png`. Take one before the action and one after. |
| `world()` | Returns the demo backend's data (`sessionStorage` key `sp.demo.world`). Use it to check that a write was stored, not only drawn. It is `null` until the first write. |
| `download(action)` | Runs `action`, saves the download into the evidence dir, returns the file name. |
| `note(text)` | Records an observed fact in `result.json`. |
| `settle()` / `quiet()` | Wait for the `main h1`, for skeletons to go, and for animations to end. |

Selectors: the UI is Indonesian by default (`locale id-ID`, time zone
`Asia/Jakarta`). Use roles and accessible names: `getByRole('button', { name: 'Tambah pengguna' })`,
`getByRole('dialog', { name: 'Ubah rute' })`, `getByLabel('Cari pengguna')`,
`getByRole('region', { name: 'Peringatan SOS' })`. Row actions are named
`<Action> — <Subject>` with an em dash, for example `Kelola — Rina Kusuma`,
`Ubah — Patroli Keliling Gudang`, `Riwayat — <guard>`. Find the exact text in
`web/src/i18n/*.id.ts`. Tabs are URL state: `/patrol?tab=routes`,
`/reports?tab=compliance`, `/settings?tab=devices`, `/incidents?tab=sos`.
The existing specs in `web/e2e/*.spec.ts` are a catalogue of working selectors.

Sample data to rely on: sites `Gudang Cikarang` and `Kantor Pusat Sudirman`; the
signed-in admin is `Rina Kusuma` (`demo.admin`); two open SOS alerts, one per
site; routes `Patroli Keliling Gudang` and `Patroli Malam Perimeter`.

A journey fails, with exit code 1, when an assertion throws **or** the page logs
a console error, an uncaught exception or a CSP violation. Do not filter those
out to get a pass.

The journeys in `journeys/` are proofs of the features in
[features/README.md](features/README.md). Write a new journey next to them when
you change a feature; put a throwaway one anywhere and pass its path.

### Evidence

Each drive writes `.claude/verify/evidence/<UTC stamp>-<name>/`:

- `NN-<label>.png` - the screenshots, in order
- `result.json` - `ok`, the journey, commit, every step with its URL, notes, errors, console problems
- `trace.zip` - open with `npx playwright show-trace <path>` from `web/`
- `world-after.json` - the demo data at the end of the run
- downloaded files, and `FAILED.png` when the journey threw

Proof standards for this repo:

- Drive the path a user takes: click the button, fill the dialog. Do not call
  the mock API, write `sessionStorage`, or set React state from the journey.
  `world()` is for reading.
- Capture the action and the result: a screenshot before, one after, and the
  state that changed (`world()`, a reopened dialog, a downloaded file).
- Open the screenshots before you claim a pass. An assertion can pass while the
  screenshot shows something else (a list that had not finished filtering yet).
- Prove a role restriction as the restricted role, not as admin.
- Report the tier. A tier 1 pass is "works in the web demo", never "works".

### Cleanup

```bash
node .claude/skills/verify/verify.mjs cleanup
```

Stops the preview server that launch started, by its recorded pid, and only
while that pid still holds the port. Removes `.claude/verify/instance-<port>/`.
Never touches `.claude/verify/evidence/`. Run it after a failed attempt too.
Do not stop servers by process name: other agents may run their own instance.

## Tier 2: backend integration tests

Not proven on the PC that generated this skill (no Go toolchain there). The
commands are the repo's documented ones:

```bash
cd backend
go build ./... && go vet ./... && go test ./...
go test -tags integration ./...                 # embedded Postgres 16, no Docker
go test -tags integration -run TestName -v ./internal/handler/...
```

Without Go installed, a portable toolchain works: unzip `go1.26.x`, put its
`bin` on `PATH`, set `GOTOOLCHAIN=local`. Evidence is the `-v` test output;
save it under `.claude/verify/evidence/<stamp>-<name>/go-test.txt`. A proof for
an API change is an integration test that sends the HTTP request and reads the
rows back, including the wrong-realm and wrong-role cases.

## Tier 3: real stack

Not proven on the PC that generated this skill (no Docker there). Treat every
step below as unverified until a run on a Docker host covers it.

```bash
cd infra && cp .env.example .env && make up     # builds, starts, waits, configures Keycloak
cd web && npx playwright test -c playwright.real.config.ts
```

The app is at `https://localhost` (self-signed CA), Keycloak at
`http://localhost:8081/auth`, realm `securepatrol-client1`. Override with
`GARDA_URL`, `KC_URL`, `GARDA_REALM`. `web/e2e/real/auth.setup.ts` signs in once
per role and holds the test accounts; `web/e2e/real/fixtures.ts` has `openAs`
for the saved sessions. Tests run serially against one shared database: do not
run two real-stack suites against the same stack, and do not point this at a
stack someone else is using. Teardown: `make down` in `infra/`.

## Helpers

| File | Invocation |
|---|---|
| `verify.mjs` | `node .claude/skills/verify/verify.mjs launch\|doctor\|drive <journey.mjs>\|cleanup [--port N]` |
| `journeys/*.mjs` | `node .claude/skills/verify/verify.mjs drive .claude/skills/verify/journeys/<name>.mjs` |

Keep the feature map current with `/maintain-verification-skill`.
