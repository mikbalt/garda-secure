# Checking that your records were not changed

*Bahasa Indonesia: [chain-verification.id.md](chain-verification.id.md)*

GARDA keeps every attendance, patrol scan and incident record of a site in a chain. Each entry
holds a fingerprint (a hash) of its record and of the entry before it, so changing or deleting
anything breaks the chain from that point on. A separate **signer** also signs the newest entry
of every site, on a key that the GARDA application cannot reach. You can check all of this
yourself, without trusting the vendor's word, with the tool `garda-verify`.

## What you need

1. **The signer key fingerprint.** 64 characters. You get it when the system is installed (on
   paper or in person). Keep it somewhere the vendor cannot change. This is the one thing you
   must hold yourself: it is what makes a forged signature useless.
2. **An export bundle** for the site and the dates you want checked (a `.zip`). Ask your GARDA
   administrator, or an admin or supervisor can download it in the dashboard API
   (`GET /api/v1/sites/<site>/chain/export?from=2026-11-01&to=2026-11-30`).
   The bundle holds the text of the records, so treat it like the records themselves.
3. **`garda-verify`**, one small file for Linux, macOS or Windows. It needs no installation, no
   internet and no account. It only reads the bundle.

## Run it

```
garda-verify --pin <fingerprint> garda-chain-1234abcd-2026-11-01_2026-11-30.zip
```

Result and exit code:

| Result | Exit code | What it means |
|---|---|---|
| `VALID` | 0 | Every entry links to the one before, every record in the bundle matches its entry, and every signed head is genuine and matches the chain. |
| `BROKEN` (record or chain) | 1 | A record or a chain entry was changed, deleted or reordered. The output names the **first** one: sequence number, kind (`attendance`, `scan`, `incident`, `incident_amendment`), record id and the reason. |
| *(usage error)* | 2 | Wrong arguments, for example no `--pin`. |
| bundle problem | 3 | The file is not a GARDA bundle, is damaged, or was changed after it was exported. |
| `BROKEN` (signed head) | 4 | A signature does not hold, the signing key is not the one you pinned (**wrong fingerprint**), or the chain no longer matches what the signer signed (the past was rewritten). |
| `UNSIGNED` | 5 | The chain has entries but no signed head at all. Nothing anchors it. |
| `STALE` | 6 | There are entries after the newest signed head and that head is older than `--max-age` (default 26 hours). Usually the signer stopped. This is not proof of tampering; ask the vendor why. |

Other options: `--json` (machine-readable result), `--max-age 48h` (or `0` to switch the check off),
`--trust-first-use` (accept the first key in the bundle when you hold no fingerprint; weaker,
because then the bundle vouches for itself).

Write down the **bundle sha256** that the tool prints. If you later get the same range again,
the same hash means the same bundle.

## What a valid result does and does not tell you

* It does tell you: in this range, nothing was changed, removed or reordered after the entries
  were signed, as far as the signer's key and the chain can show. A record that was changed in the database and then
  re-chained (so that it looks normal) is still caught, because the signed heads of the past no longer match.
* A record changed **through the application** (a supervisor's override, a clock-out) is not tampering: it adds a new entry and
  the old entries stay. The chain shows that history instead of hiding it.
* Older states of a record are not kept, so for a record that changed again after your date range, the
  tool checks its entry as a link of the chain only. The output says how many (`records_not_checked`).
* Entries after the newest signed head (up to an hour or so) are not signed yet. They are checked as links, and `STALE` appears if the
  signer has been silent for too long.
* If the signer's key itself was lost or leaked, the vendor starts a new epoch and you must pin the new
  fingerprint (given to you again out of band). Until then the tool reports exit code 4.

## Online check (no tool needed)

`GET /api/v1/sites/<site>/chain/verify?from=2026-11-01&to=2026-11-30&pin=<fingerprint>` answers
`{"valid": true, ...}` or the first broken record, for admin, supervisor and client-readonly users. It
starts from the last signed head before your range, so a month is quick. If the range is too big or the check
takes too long it answers `verify_too_large`: use the bundle and `garda-verify` instead. The online check trusts the
server you ask; the offline check does not, which is why you should use it for anything serious.

## Bundle format (`garda-chain-bundle/v1`)

A zip with these files:

| File | Content |
|---|---|
| `manifest.json` | `format`, `site_id`, `site_timezone`, `from`/`to` (requested range, UTC, end not included), `exported_at`, `anchor_seq` (0 = starts at the first entry of the site; otherwise the seq of a signed head in `heads.json` that the first entry continues from), `first_seq`, `last_seq`, `entries`, `heads`, `chain_tip_seq`/`chain_tip_hash` (the chain's head when exported), `files` (sha256 of the three files below). The sha256 of `manifest.json` itself is the bundle's integrity hash. |
| `entries.jsonl` | one entry per line: `seq`, `kind`, `record_id`, `record_hash`, `prev_hash`, `entry_hash` (hex), `hash_version` (1), `origin`, `created_at` (UTC, microseconds), `record_state` (`latest`, `superseded` with `next_seq`, or `deleted`), and for `latest` the exact `record` text. |
| `heads.json` | signed heads from `anchor_seq` to `last_seq`, the same fields as `signer export`: `site_id`, `seq`, `entry_hash`, `signed_at`, `reason`, `key_id`, `signature`. |
| `keyring.json` | the signer's public keys with their handover signatures, as in `signer export`. **No private key is ever in a bundle.** |

How to recompute (any language):

* `record_hash = sha256("<kind>:" + record)`, for `record_state = latest`.
* `entry_hash = sha256(version|site_id|seq|kind|record_id|record_hash|prev_hash|origin|created_at)` with the
  hashes as lowercase hex and `created_at` as `YYYY-MM-DDTHH:MM:SS.ffffff` (UTC, no `Z`), parts joined with `|`.
* The first entry links to `sha256("garda-record-chain:v1:" + site_id)` (or to the signed head at `anchor_seq`); every other entry's `prev_hash` is the
  `entry_hash` before it, and `seq` has no gaps.
* A head is `ed25519` over `garda-head:v1|site|seq|entry_hash hex|signed_at|key_id` (times UTC with 6 decimals, `Z`).
  A key counts only if its `key_id` (sha256 of the public key) is the fingerprint you pinned, or is reached from it by
  valid handovers in `keyring.json`.

A range starts at the newest signed head before it and ends at the first signed head at or after it,
so that both ends are anchored by a signature and nothing can be missing in between.
