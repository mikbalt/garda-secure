/**
 * Model of one SOS delivery row (notification_log, lane "sos", a rule channel:
 * e-mail or WhatsApp) and the workers that send it.
 *
 * Every action below is ONE indivisible step in the code: one SQL statement, or
 * one outcome of the send. The comment on each action names the code it stands
 * for. If that code changes, change the action; see SKILL.md.
 *
 *   backend/internal/repository/notification_outbox.go   ClaimDelivery, MarkDelivery*
 *   backend/internal/notification/worker.go              deliver, RetryDelay, pgStore.MarkFailed, sosUndelivered
 *
 * Row fields:     status (queued|sent|failed), attempts, due (next_attempt_at <= now),
 *                 leased (locked_until >= now)
 * Worker fields:  pc (where it is inside claimAndSend/deliver), seen (d.Attempts read by the claim)
 * History:        everSent (some channel Send returned nil), opsAlert (ops_sos_undelivered staged)
 */

/**
 * @param {object} cfg
 * @param {string[]} cfg.workers        dispatch goroutines, on any number of replicas
 * @param {number}  cfg.maxAttempts     stands for sosMaxAttempts (33 in the code)
 * @param {boolean} cfg.alertInFailTx   the ops alert is staged in the transaction of MarkDeliveryFailed.
 *                                      true = the code today (worker.go: pgStore.MarkFailed runs both in one pgx.BeginFunc).
 * @param {boolean} cfg.stallBeyondLease a live worker can still be inside deliver() after its lease ran out.
 *                                      false = the code's premise: leaseFor > sendTimeout + dbTimeout.
 * @param {'none'|'mark_sent_on_error'} cfg.bug   a deliberately broken variant, to show a property can fail
 */
export function makeModel(cfg) {
  const { workers, maxAttempts } = cfg
  const idle = (s) => workers.every((w) => s.pc[w] === 'idle')

  const init = {
    status: 'queued', // InsertDelivery (Notifier.Stage), committed with the sos_alerts row
    attempts: 0,
    due: true,
    leased: false,
    everSent: false,
    opsAlert: false,
    pc: Object.fromEntries(workers.map((w) => [w, 'idle'])),
    seen: Object.fromEntries(workers.map((w) => [w, 0])),
  }

  const at = (s, w, pc, more = {}) => ({ ...s, ...more, pc: { ...s.pc, [w]: pc } })

  /** All steps possible from state s: [{ label, next }]. */
  function steps(s) {
    const out = []
    const add = (label, next) => out.push({ label, next })

    for (const w of workers) {
      const pc = s.pc[w]

      // ClaimDelivery: one UPDATE ... WHERE status='queued' AND next_attempt_at<=now()
      // AND (locked_until IS NULL OR locked_until<now()) ... FOR UPDATE SKIP LOCKED.
      if (pc === 'idle' && s.status === 'queued' && s.due && !s.leased) {
        add(`${w}: ClaimDelivery leases the row (reads attempts=${s.attempts})`, {
          ...at(s, w, 'sending', { leased: true }),
          seen: { ...s.seen, [w]: s.attempts },
        })
      }

      // deliver: w.send(...) returns nil or an error (bounded by sendTimeout).
      if (pc === 'sending') {
        add(`${w}: channel Send succeeds (the recipient gets the SOS)`, at(s, w, 'sendOk', { everSent: true }))
        add(`${w}: channel Send fails`, at(s, w, 'sendErr'))
      }

      // deliver, sendErr == nil: MarkDeliverySent (status='sent', attempts+1, locked_until=NULL).
      // The UPDATE is WHERE id=$1 only: it does not check status or the lease.
      const marksSent = pc === 'sendOk' || (cfg.bug === 'mark_sent_on_error' && pc === 'sendErr')
      if (marksSent) {
        add(`${w}: MarkDeliverySent`, at(s, w, 'idle', { status: 'sent', attempts: s.attempts + 1, leased: false }))
        add(`${w}: MarkDeliverySent fails (database error, logged); deliver returns`, at(s, w, 'idle'))
      }

      if (pc === 'sendErr' && cfg.bug !== 'mark_sent_on_error') {
        // RetryDelay(LaneSOS, d.Attempts+1): gives up when attempts >= sosMaxAttempts.
        const givesUp = s.seen[w] + 1 >= maxAttempts
        if (!givesUp) {
          // MarkDeliveryRetry: attempts+1, next_attempt_at=later, locked_until=NULL. Status is not touched.
          add(`${w}: MarkDeliveryRetry (next attempt later)`, at(s, w, 'idle', { attempts: s.attempts + 1, due: false, leased: false }))
          add(`${w}: MarkDeliveryRetry fails (database error, logged); deliver returns`, at(s, w, 'idle'))
        } else if (cfg.alertInFailTx) {
          // pgStore.MarkFailed: MarkDeliveryFailed and Notifier.Stage(ops_sos_undelivered) in one
          // transaction. On an error deliver returns; the row stays queued and leased.
          add(`${w}: MarkDeliveryFailed and the ops alert commit together`, at(s, w, 'idle', { status: 'failed', attempts: s.attempts + 1, leased: false, opsAlert: true }))
          add(`${w}: MarkDeliveryFailed with the ops alert fails (database error); deliver returns`, at(s, w, 'idle'))
        } else {
          // The broken variant (the code before D-064): MarkDeliveryFailed, then the ops alert in a
          // second transaction, attempted whether or not the first UPDATE worked.
          add(`${w}: MarkDeliveryFailed (the delivery gives up)`, at(s, w, 'alerting', { status: 'failed', attempts: s.attempts + 1, leased: false }))
          add(`${w}: MarkDeliveryFailed fails (database error, logged)`, at(s, w, 'alerting'))
        }
      }

      // Only in the broken variant: the ops alert in its own transaction.
      if (pc === 'alerting') {
        add(`${w}: alertSOSUndelivered stages the ops alert`, at(s, w, 'idle', { opsAlert: true }))
        add(`${w}: alertSOSUndelivered fails (Publish error, logged only)`, at(s, w, 'idle'))
      }

      // The process dies anywhere inside claimAndSend/deliver (kill, OOM, power loss).
      // A graceful shutdown is not this step: Run waits for deliver to finish.
      if (pc !== 'idle') add(`${w}: the worker process dies`, at(s, w, 'idle'))
    }

    // locked_until passes. With the code's timing premise a live worker is done before
    // that, so the lease only runs out on a row nobody is working on.
    if (s.leased && (cfg.stallBeyondLease || idle(s))) add('the lease runs out (locked_until < now)', { ...s, leased: false })

    // next_attempt_at passes.
    if (!s.due) add('the retry delay passes (next_attempt_at <= now)', { ...s, due: true })

    return out
  }

  return { init, steps }
}

/**
 * The properties. `state` ones must hold in every reachable state; `step` ones
 * on every step. Each has the promise and the harm in one plain sentence.
 */
export const properties = {
  SentMeansSent: {
    promise: 'A delivery is recorded as sent only after a channel really accepted the message.',
    harm: 'The dashboard shows an SOS as delivered while nobody was told, and nothing retries it.',
    state: (s) => s.status !== 'sent' || s.everSent,
  },
  NoSilentLoss: {
    promise: 'An SOS delivery that gives up always leaves an ops alert (or was in fact delivered).',
    harm: 'A guard presses SOS, every attempt fails, and no person is ever told that the alarm did not go out.',
    state: (s, cfg) => !(s.status === 'failed' && cfg.workers.every((w) => s.pc[w] !== 'alerting')) || s.opsAlert || s.everSent,
  },
  OneSenderAtATime: {
    promise: 'While one worker is sending a delivery, no other worker sends the same one.',
    harm: 'Recipients get the same SOS several times, and two workers write conflicting outcomes for one delivery.',
    state: (s, cfg) => cfg.workers.filter((w) => ['sending', 'sendOk', 'sendErr'].includes(s.pc[w])).length <= 1,
  },
  TerminalIsStable: {
    promise: 'Once a delivery is sent or failed, no worker changes that outcome.',
    harm: 'A delivered SOS flips to failed (false ops alarm) or a failed one flips to sent (the failure is hidden).',
    step: (a, b) => a.status === 'queued' || a.status === b.status,
  },
}
