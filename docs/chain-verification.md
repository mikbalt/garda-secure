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
garda-verify --pin <fingerprint> --expect-from 2026-11-01 --expect-to 2026-11-30 garda-chain-1234abcd-2026-11-01_2026-11-30.zip
```

Always give `--expect-from` and `--expect-to` (the first and last day you asked for, in the site's
time zone). Without them the tool only knows the range the person who made the bundle *says* it
covers: the tool then ends with `RANGE NOT AUTHENTICATED` (exit 11), never with `VALID`. Use an end date that is
at least a day old: the bundle has to end at a signed head made after your range, so an expected range that ends
today or later can never pass (a head cannot be signed after the end of today). Only if you really cannot give a
range, add `--no-range-check`: the result is then printed as `VALID (range not checked)`.

Result and exit code:

| Result | Exit code | What it means |
|---|---|---|
| `VALID` | 0 | The entries in the bundle link to each other, the records it carries match their entries, and its signed heads are genuine and match the chain. Read "What VALID does not prove" below before you rely on it. |
| `BROKEN` (record or chain) | 1 | A record or a chain entry was changed, deleted or reordered. The output names the **first** one: sequence number, kind (`attendance`, `scan`, `incident`, `incident_amendment`), record id and the reason. |
| *(usage error)* | 2 | Wrong arguments, for example no `--pin`. |
| bundle problem | 3 | The file is not a GARDA bundle, is damaged, does not match its own file hashes, or its range does not fit its entries. (The file hashes only catch damage and careless edits.) |
| `BROKEN` (signed head) | 4 | A signature does not hold, the signing key is not the one you pinned (**wrong fingerprint**), or the chain no longer matches what the signer signed (the past was rewritten). |
| `UNSIGNED` | 5 | The chain has entries but no signed head at all. Nothing anchors it. |
| `STALE` | 6 | There are entries after the newest signed head and that head is older than `--max-age` (default 26 hours). Usually the signer stopped. This is not proof of tampering; ask the vendor why. |
| `UNSIGNED TAIL` | 7 | The last entries of the bundle come after the newest signed head, so no signature vouches for them. A bundle that was cut short looks exactly like this. Export again after the next signed head (they are made about hourly). **Not valid.** |
| `UNVERIFIED RECORDS` | 8 | Some records are said to have changed again after the bundle, and the later entry is not in it, so their text cannot be checked. Export a range that includes the change. `--allow-unverified-records` changes the answer to exit 12 (`UNVERIFIED RECORDS ACCEPTED`), which is **still not valid**: the output says how many entries, and what share, were not checked. |
| `NO ENTRIES` | 9 | The bundle has no entry. It proves nothing about the range: a signature cannot show that a site had no entries. **Not valid.** |
| `RANGE MISMATCH` | 10 | The bundle does not cover the range you gave with `--expect-from` / `--expect-to`: it was made for other dates, it starts from a head signed after your range began, or its newest signed head is older than the end of your range. **Not valid.** |
| `RANGE NOT AUTHENTICATED` | 11 | The chain and the signatures hold, but you gave no `--expect-from` / `--expect-to`, so the range is only what the exporter says (a bundle cut at an older signed head, started late, or an old genuine one looks the same). Give the range, or accept it knowingly with `--no-range-check` (exit 0, printed as `VALID (range not checked)`). **Not valid.** |
| `UNVERIFIED RECORDS ACCEPTED` | 12 | As 8, but you passed `--allow-unverified-records`. Not exit 0 on purpose: a flag added out of habit must not turn it into a pass. |

Besides the result the tool prints when the newest signed head was signed and when the bundle was exported
(by the exporter's clock), and warns when the bundle is older than `--max-age`: an old bundle is still
"valid" for what it holds, but it says nothing about what happened after it was made. Other options: `--json` (machine-readable result), `--max-age 48h` (or `0` to switch the check off),
`--trust-first-use` (accept the first key in the bundle when you hold no fingerprint; weaker,
because then the bundle vouches for itself).

Write down the **bundle sha256** that the tool prints. If you later get the same range again,
the same hash means the same bundle.

## What a valid result tells you

* In this bundle, no entry was changed, removed or reordered after it was signed, as far as the signer's key and the
  chain can show. A record that was changed in the database and then re-chained (so that it looks normal) is still caught,
  because the signed heads of the past no longer match.
* A record changed **through the application** (a supervisor's override, a clock-out) is not tampering: it adds a new entry and
  the old entries stay. The chain shows that history instead of hiding it.
* Older states of a record are not kept, so for a record that changed again inside the bundle, the
  tool checks its old entry as a link of the chain only.
* If the signer's key itself was lost or leaked, the vendor starts a new epoch and you must pin the new
  fingerprint (given to you again out of band). Until then the tool reports exit code 4.

## What VALID does NOT prove

`VALID` is a strong statement about the entries that are in the bundle. It is not a statement about everything that
happened. In particular it does **not** prove:

* **That nothing is missing.** A bundle that ends early, or leaves out the last records, can still be `VALID` if it
  ends exactly at a signed head. Give `--expect-to` (the tool then needs a signed head made *after* your range) and
  check that the entry counts look right for the period. Entries after the newest signed head are reported as
  `UNSIGNED TAIL`, never as valid.
* **That the dates are the ones you asked for.** The range written in the bundle is the claim of whoever made it.
  Only `--expect-from` and `--expect-to` check it against signing times.
* **Records that changed again outside the bundle.** For those the text is not checked (exit 8 unless you opt out).
* **That the bundle is current.** An old, genuine bundle stays valid forever. Look at "Exported" and "newest signed head".
* **That nobody with full access to the server and to the signer's machine acted.** Someone who controls the host as
  root *and* the signer's key volume can sign a rewritten history. Pinning the fingerprint, keeping your own earlier
  bundles and signing key handovers in the presence of the client are what limit this.
* **That a site that has no entries had none.** An empty bundle is exit 9, not valid.
* **That no one saw the data.** Exports are not logged yet; the bundle holds personal data.
* **That the file hashes in the manifest protect the bundle.** They only show damage or a careless edit. The chain and
  the signed heads are what protect it.

## Online check (no tool needed)

`GET /api/v1/sites/<site>/chain/verify?from=2026-11-01&to=2026-11-30&pin=<fingerprint>` answers
`{"valid": true, "valid_pinned": true, ...}` or the first broken record, for admin, supervisor and client-readonly users. It
starts from the last signed head before your range, so a month is quick. If the range is too big or the check
takes too long it answers `verify_too_large`: use the bundle and `garda-verify` instead. The online check trusts the
server you ask; the offline check does not, which is why you should use it for anything serious. Without `pin` the
answer is `"valid": null` with `"trust": "unverified_trust"` (the server vouches for itself), never `true`.
A user can have one check running and a few per minute. **Read `status`, not `valid`**: `valid` is `false` for every result that is not
a clean pass, which includes `unsigned_tail`, `unverified_records` and `no_entries` (not broken, but not proven), and `null` without a pin.
Show `status` and `first_break` to people. The online check takes the requested dates as given, so it does not run the expected-range check.

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

A range starts at the newest signed head that was signed before the range began and ends at the first signed head
that was signed after the range ended (or at the chain's end), so that both ends are anchored by a signature and
nothing can be missing in between. See "What VALID does NOT prove" for what that still leaves open.
