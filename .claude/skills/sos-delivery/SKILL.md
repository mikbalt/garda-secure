---
name: sos-delivery
description: Formal check of GARDA's SOS notification delivery - the notification_log outbox row, its lease, retries, give-up and the ops alert (backend/internal/notification/worker.go, backend/internal/repository/notification_outbox.go). Explores every order of worker steps, crashes and database errors in a small model. Use before and after any change to ClaimDelivery, MarkDeliverySent/Retry/Failed, Worker.deliver, pgStore.MarkFailed, RetryDelay, or the constants sendTimeout, leaseFor, dbTimeout, sosMaxAttempts; and when asked whether an SOS can be lost, sent twice or stuck.
---

# SOS delivery: formal check

When a guard presses SOS, the API stores the alert and one `notification_log`
row per recipient in one transaction. Worker goroutines, on one or more
replicas, then claim each row with a lease, send it, and record the outcome.
Tests cover the paths someone wrote down. This check covers **every order** in
which three workers, crashes, database errors, lease expiry and retry delays can
interleave on one delivery.

## Run it

From the repository root. Needs Node 18+ and git, nothing else. Takes under a second.

```bash
node .claude/skills/sos-delivery/check.mjs
```

| Exit | Meaning | What to do |
|---|---|---|
| 0 | Every scenario has its expected outcome and the covered code is the reviewed one. | Nothing. |
| 1 | A scenario differs from its expectation. | Read the trace. See "Real bug or modelling error". |
| 2 | Outcomes are as expected, but the covered code changed since the model was compared with it. | See "When the covered code changed". Not a pass. |

## Does my change touch it?

Yes if the diff touches any of these (also listed in [coverage.json](coverage.json)):

| File | Covered |
|---|---|
| `backend/internal/repository/notification_outbox.go` | `ClaimDelivery`, `MarkDeliverySent`, `MarkDeliveryRetry`, `MarkDeliveryFailed` |
| `backend/internal/notification/worker.go` | `RetryDelay`, `Worker.claimAndSend`, `Worker.deliver`, `pgStore.MarkFailed`, `sosUndelivered`, constants `sendTimeout`, `leaseFor`, `dbTimeout`, `sosMaxAttempts` |

`check.mjs` hashes both files and reports `DRIFT` when either differs from the
reviewed version. A change elsewhere in those files (gauges, wake-ups,
`SendTest`) also trips the hash; it needs a look and `--accept`, no model change.

Also relevant, not hashed: a new migration that changes `notification_log`
columns `status`, `attempts`, `next_attempt_at`, `locked_until`, and any new
code that writes those columns.

## The properties

| Property | Promise | Harm if broken | On today's code |
|---|---|---|---|
| `SentMeansSent` | A delivery is recorded as sent only after a channel really accepted the message. | The dashboard shows an SOS as delivered while nobody was told, and nothing retries it. | holds |
| `OneSenderAtATime` | While one worker is sending a delivery, no other worker sends the same one. | Duplicate SOS messages, and two workers write conflicting outcomes. | holds |
| `TerminalIsStable` | Once a delivery is sent or failed, no worker changes that outcome. | A delivered SOS flips to failed, or a failed one to sent. | holds |
| `NoSilentLoss` | An SOS delivery that gives up always leaves an ops alert (or was in fact delivered). | Every attempt fails and no person is ever told the alarm did not go out. | holds (since D-064) |

### Finding F1 (fixed, D-064)

Until D-064 `Worker.deliver` ran `MarkDeliveryFailed` and then the ops alert as
two transactions. When the alert failed, or the process died in between, the
delivery ended `failed` with no `ops_sos_undelivered` alert and nothing claimed
it again. Now `pgStore.MarkFailed` commits both in one transaction; when it
fails the row stays queued and is attempted again after the lease. Scenario
`mutant-alert` keeps the old shape alive as a broken variant, so the check still
shows that `NoSilentLoss` can fail. The integration test
`TestUndeliveredSOSFailsOnlyWithItsOpsAlert` covers the same thing on real SQL.

The alert is only staged when ops alert rules exist. With none configured,
`stageOps` logs a warning and the failure is silent again: that is
configuration, outside this model.
## What is checked, and what is not

Checked, exhaustively: one SOS delivery on a rule channel (e-mail or WhatsApp),
three workers, giving up at attempt 3. Every step is one SQL statement or one
send outcome. Any worker can die at any step. Any `Mark*` (the give-up with its ops alert included) can fail with a database error. The lease and the retry delay can run
out at any allowed moment.

Assumed, and read from the code on every run: a live worker finishes before its
lease ends, that is `leaseFor > sendTimeout + dbTimeout`. `check.mjs` evaluates
those constants from `worker.go`. If the inequality stops holding, it models
workers that outlive their lease, and `OneSenderAtATime` breaks. The model does
not cover a process that is frozen for longer than the 20 s margin (VM pause,
long GC stall): `mutant-lease` shows what happens then. `Mark*` update by
`id` only and do not check the lease.

Not covered:

- The bound. Three workers and three attempts, not 33 attempts or N workers. Nothing in the model depends on the count beyond "not the last attempt / the last attempt".
- Liveness. The check shows nothing bad is recorded; it does not show a delivery finishes within a time.
- Staging: that `createSos` commits the alert and its deliveries together (`handler/incidents.go`, `Notifier.Stage`).
- Push channels (best effort by design, no ops alert), the normal lane, the admin requeue (`RequeueNotification`), and the delivery of the ops alert itself.
- Whether the SQL does what the model says. That is the job of `go test -tags integration ./...` in `backend/`.
- Clock skew between replicas and Postgres.

A pass is a statement about [model.mjs](model.mjs), which is a hand-written
description of the code. It is as true as that description.

## Reading a result

Each line is `scenario property outcome`. A trace is the shortest sequence of
steps that breaks the property, in the code's words (`w1: MarkDeliveryFailed`).
Retell it to the user as those steps; they do not need the model.

| Scenario | Models | Expected |
|---|---|---|
| `code` | the code as it is | all four properties hold |
| `mutant-alert` | broken on purpose: ops alert staged after `MarkDeliveryFailed`, in its own transaction | `NoSilentLoss` breaks |
| `mutant-lease` | broken on purpose: a lease shorter than a send | `OneSenderAtATime`, `TerminalIsStable` break |
| `mutant-mark` | broken on purpose: `MarkDeliverySent` after a failed send | `SentMeansSent` breaks |

The mutants exist to show that each property can fail. If a mutant reports
`UNEXPECTED ... holds`, the property stopped guarding anything: the model or the
property was weakened. Fix that before trusting any pass.

## Real bug or modelling error

When `code` breaks a property, walk the trace against the code:

1. For each step, open the function it names. Is that step possible there, exactly as written? `model.mjs` has a comment on every action naming its code.
2. Is each step really indivisible in the code? One SQL statement is. Two statements outside a transaction are two steps.
3. Every step real: it is a bug in the code. Report the trace. Do not relax the property or delete the step.
4. A step is not possible in the code: the model is wrong. Fix that action in `model.mjs`, say what you changed and why, and rerun.

Never make a failing check pass by weakening a property, lowering a bound, or
removing `the worker process dies` or a `fails (database error)` step.

## When the covered code changed

1. Read the diff of the covered files against the reviewed commit: `git diff <syncCommit from coverage.json> -- backend/internal/notification/worker.go backend/internal/repository/notification_outbox.go`.
2. Map it to the model:
   - A changed `WHERE` or `SET` in `ClaimDelivery` or a `Mark*`: change the guard or the effect of that action in `steps()`.
   - A reordered or new step in `deliver`: change the `pc` flow. Two database calls that are now one transaction become one action; one that was split becomes two.
   - The ops alert leaves the transaction of `MarkDeliveryFailed` (`pgStore.MarkFailed`): set `alertInFailTx: false` in `code` in `check.mjs`. `NoSilentLoss` then breaks, which is the real result.
   - A changed timeout or `sosMaxAttempts`: nothing to edit, the run reads them.
   - A change outside the covered functions: no model change.
3. Run the check. Resolve any exit 1 as in "Real bug or modelling error".
4. Only when the model describes the code at your change, record it:

```bash
node .claude/skills/sos-delivery/check.mjs --accept
```

That writes the file hashes and the current commit (`syncCommit`) into
`coverage.json`. Commit `coverage.json` with the code change. Do not run
`--accept` to silence `DRIFT` without doing steps 1 to 3.

The commit the model was last confirmed against is `syncCommit` in `coverage.json`.

## Files

| File | Role |
|---|---|
| `model.mjs` | The model: row and worker state, one action per code step, the four properties. |
| `check.mjs` | Breadth-first search over every reachable state, the scenarios, the timing premise, drift detection. |
| `coverage.json` | Covered files and functions, their reviewed hashes, the sync commit. |

The check is a small explicit-state model checker in plain JavaScript, so it
runs wherever the web app builds, with no download. The same model can be
written in TLA+ if a team wants TLC; read TLA+ results with the pstack
`/statechart` skill.
