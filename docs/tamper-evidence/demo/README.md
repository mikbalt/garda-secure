# Demo: an edited record is caught

`demo.sh` runs against a **running, disposable or demo stack** (not production). It:

1. seeds a new site, `GARDA DEMO <time>`, with 30 days of attendance, patrol scans and a few incidents (93 chain entries), written the way the app
   writes them (through `chain_append`);
2. asks the signer to sign the newest entry;
3. exports the bundle through the API and runs `garda-verify` with the pinned fingerprint: **exit 0, `VALID (range not checked)`**;
4. edits one attendance record **directly in the database as the owner role** and checks again: **exit 1, `BROKEN`, "record modified"**, naming
   the sequence number, the kind and the record id; the online check agrees;
5. puts the record back and checks again (exit 0), so no broken site is left behind;
6. changes a record inside a copy of the exported file: **exit 3** (the file no longer matches its hash); and again with the file hash in the
   manifest corrected as well: **exit 1**.

The script refuses to run unless `DEMO_I_AM_ON_A_DISPOSABLE_STACK=1` is set (never production).
The demo site belongs to `DEMO_REALM`: every user of that realm can see it, and the scheduled job makes monthly proof packs for it. Use a demo realm only.

It is repeatable: every run makes a new site, and the temporary files are removed on exit. The edit is reverted on any exit, also on an error.
The demo site stays in the database, on purpose: the chain and the signed heads are append-only, and a real signer remembers what it signed.
`DEMO_PURGE=1` deletes the demo site, its records, chain and heads at the end. **Use it only on a disposable database** (it switches the
append-only guards off as the owner, inside one transaction). It leaves the signer's own file holding heads that the database no longer has, so the
signer refuses that site and its health goes red (D-071) unless the signer volume is reset with the stack.

## Needs

`bash`, `curl`, `python3` (standard library only), `psql`, and the backend code (the script builds `garda-verify` with Go if it is not on
`PATH`; set `GARDA_VERIFY` to use another binary). A stack with migration 014 or later (the chain export): older stacks stop at step 0.

```
export DEMO_I_AM_ON_A_DISPOSABLE_STACK=1             # required
export DEMO_API=https://garda.example/api/v1       # base URL incl. /api/v1
export DEMO_TOKEN=...                              # bearer token of an admin or supervisor in DEMO_REALM
export DEMO_REALM=securepatrol-client1             # realm that owns the demo site
export DEMO_DB_URL='postgres://<owner>:<pw>@host:5432/<db>'   # the OWNER role (APP_DB_USER), not sp_runtime
export DEMO_PIN=<64 hex>                           # signer fingerprint from installation (`make signer-key` printed it)
export DEMO_SIGN_URL=http://signer:9092            # signer POST /sign?site_id=... (internal network only)
docs/tamper-evidence/demo/demo.sh
```

With Docker and no local `psql`: `PSQL="docker run --rm -i postgres:16.15-alpine psql"` (then the host in `DEMO_DB_URL` must be reachable from the container).
The script calls the signer with `curl` from where it runs, so that machine must reach `DEMO_SIGN_URL`.

## Why `--no-range-check`

The chain stamps entries with the time they are written, so seeded entries are dated today. A strict check (`--expect-from`, `--expect-to`)
needs a signed head made after the last day of the range, so it can only pass from tomorrow on, when the signer has signed after the day ended:

```
garda-verify --pin $DEMO_PIN --expect-from <first day> --expect-to <the seeding day> garda-chain-....zip
```

## What was run for TAQ-23

See the pull request. The shared Docker stack was at migration 13 with no export endpoint, so the script was run against the Go
integration harness (embedded Postgres 16, the real router, a real signer with a key file, as the owner role `postgres`), with the output quoted
there.
