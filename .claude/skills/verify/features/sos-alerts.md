# SOS alerts

When a guard presses SOS on a tablet, every supervisor and administrator sees a red banner on every page, for every site, until someone acknowledges the alert.

## Sub-features

- `sos-banner` shows the open alerts of all sites, whichever site is selected.
- `sos-acknowledge` marks one alert as handled and removes it from the banner.
- `sos-sound` toggles the alarm sound with a pressed-state button.
- `sos-history` lists alerts on the `SOS` tab of the incidents page.

## How to get to it (user POV)

- Open any page as `admin` or `supervisor`: the banner `Peringatan SOS` is at the top.
- Choose `Insiden` in the main navigation, then the tab `SOS` (`/incidents?tab=sos`).
- Use the keyboard only: Tab to a `Tanggapi` button and press Enter.

## Driving it with verify.mjs

Preconditions:

- `DOCTOR OK` on the instance.
- The sample data has two open alerts, one at `Gudang Cikarang` and one at `Kantor Pusat Sudirman`.

Run `node .claude/skills/verify/verify.mjs drive .claude/skills/verify/journeys/sos-acknowledge.mjs`.

- **See the banner.** `openAs('supervisor', '/')`. The region `Peringatan SOS` shows both site names and two `Tanggapi` buttons.
- **Acknowledge.** Click the first `Tanggapi`. The text `SOS ditanggapi.` appears.
- **Proof.** The banner has one `Tanggapi` button fewer; screenshots `01` and `02` show the banner before and after.

## Gotchas

- `openAs` mutes the alarm (`sp.sos.muted`). To prove the sound toggle, check the button `Suara mati` and its `aria-pressed` value, not audio.
- The banner is site-independent on purpose. Switching the site selector must not change it.
- `hr` and `client-readonly` get no banner. Prove that as those roles.
- Not driven: `sos-sound`, `sos-history`, the keyboard-only entry point. See `web/e2e/keyboard.spec.ts` and `web/e2e/features.spec.ts`.
- In the real stack the alert arrives from a tablet and escalates through notifications. The demo proves only the dashboard side.
