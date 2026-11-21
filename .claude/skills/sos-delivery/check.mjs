#!/usr/bin/env node
/**
 * Exhaustive check of the SOS delivery model (model.mjs) against its properties.
 *
 *   node .claude/skills/sos-delivery/check.mjs            run every scenario, compare with the expected outcome
 *   node .claude/skills/sos-delivery/check.mjs --accept   record the covered code as reviewed (after you compared it with the model)
 *
 * Exit 0: every scenario has its expected outcome and the covered code is the reviewed one.
 * Exit 1: a scenario differs from its expectation (a real result: read the trace).
 * Exit 2: only drift - the covered code changed since the model was last compared with it.
 *
 * No dependencies: Node 18+ and git.
 */
import { spawnSync } from 'node:child_process'
import crypto from 'node:crypto'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { makeModel, properties } from './model.mjs'

const HERE = path.dirname(fileURLToPath(import.meta.url))
const REPO = path.resolve(HERE, '..', '..', '..')
const COVERAGE = path.join(HERE, 'coverage.json')
const coverage = JSON.parse(fs.readFileSync(COVERAGE, 'utf8'))
const read = (rel) => fs.readFileSync(path.join(REPO, rel), 'utf8').replace(/\r\n/g, '\n')
const sha = (rel) => crypto.createHash('sha256').update(read(rel)).digest('hex')

// ── Facts read from the real code ───────────────────────────────────────────

/** Evaluates a Go duration constant of worker.go in seconds. */
function goSeconds(src, name, seen = {}) {
  const m = new RegExp(`^\\s*${name}\\s*=\\s*(.+?)\\s*(?://.*)?$`, 'm').exec(src)
  if (!m) throw new Error(`constant ${name} not found in worker.go: update goSeconds/codeFacts in check.mjs`)
  let expr = m[1].replace(/time\.Second/g, '1').replace(/time\.Minute/g, '60')
  expr = expr.replace(/[A-Za-z_]\w*/g, (id) => String((seen[id] ??= goSeconds(src, id, seen))))
  if (!/^[\d\s+*()-]+$/.test(expr)) throw new Error(`cannot evaluate ${name} = ${m[1]}: update goSeconds in check.mjs`)
  return Function(`"use strict"; return (${expr})`)()
}

function codeFacts() {
  const src = read('backend/internal/notification/worker.go')
  const sendTimeout = goSeconds(src, 'sendTimeout')
  const dbTimeout = goSeconds(src, 'dbTimeout')
  const leaseFor = goSeconds(src, 'leaseFor')
  const max = /^\s*sosMaxAttempts\s*=\s*(\d+)/m.exec(src)
  if (!max) throw new Error('sosMaxAttempts not found in worker.go: update codeFacts in check.mjs')
  // A live worker holds a row for at most one send plus one bookkeeping query.
  return { sendTimeout, dbTimeout, leaseFor, sosMaxAttempts: Number(max[1]), leaseOutlivesWork: leaseFor > sendTimeout + dbTimeout }
}

// ── Explicit-state checker (breadth first, so a trace is a shortest one) ────

function check(cfg, propName) {
  const prop = properties[propName]
  const model = makeModel(cfg)
  const key = JSON.stringify
  const parent = new Map([[key(model.init), null]])
  const queue = [model.init]
  const trace = (k, last) => {
    const lines = []
    for (let cur = k; parent.get(cur); cur = parent.get(cur).from) lines.unshift(parent.get(cur).label)
    if (last) lines.push(last)
    return lines
  }
  let transitions = 0
  for (let i = 0; i < queue.length; i++) {
    const s = queue[i]
    const k = key(s)
    if (prop.state && !prop.state(s, cfg)) return { ok: false, states: parent.size, trace: trace(k), end: s }
    for (const { label, next } of model.steps(s)) {
      transitions++
      if (prop.step && !prop.step(s, next, cfg)) return { ok: false, states: parent.size, trace: trace(k, label), end: next }
      const nk = key(next)
      if (!parent.has(nk)) {
        parent.set(nk, { from: k, label })
        queue.push(next)
      }
    }
  }
  return { ok: true, states: parent.size, transitions }
}

// ── Scenarios ───────────────────────────────────────────────────────────────

const facts = codeFacts()
const bounds = { workers: ['w1', 'w2', 'w3'], maxAttempts: 3 }
/** The code as it is today. Change these knobs when the code changes (SKILL.md). */
const code = { ...bounds, alertInFailTx: true, stallBeyondLease: !facts.leaseOutlivesWork, bug: 'none' }

const scenarios = [
  { name: 'code', what: 'the code as it is', cfg: code, props: Object.keys(properties), expect: 'hold' },
  { name: 'mutant-alert', what: 'broken on purpose: ops alert staged after MarkDeliveryFailed, in its own transaction', cfg: { ...code, alertInFailTx: false }, props: ['NoSilentLoss'], expect: 'break' },
  { name: 'mutant-lease', what: 'broken on purpose: lease shorter than a send', cfg: { ...code, stallBeyondLease: true }, props: ['OneSenderAtATime', 'TerminalIsStable'], expect: 'break' },
  { name: 'mutant-mark', what: 'broken on purpose: MarkDeliverySent after a failed send', cfg: { ...code, bug: 'mark_sent_on_error' }, props: ['SentMeansSent'], expect: 'break' },
]

function drift() {
  return coverage.files.filter((f) => !fs.existsSync(path.join(REPO, f.path)) || sha(f.path) !== f.sha256).map((f) => f.path)
}

if (process.argv.includes('--accept')) {
  const head = spawnSync('git', ['rev-parse', 'HEAD'], { cwd: REPO, encoding: 'utf8' }).stdout.trim()
  coverage.syncCommit = head
  for (const f of coverage.files) f.sha256 = sha(f.path)
  fs.writeFileSync(COVERAGE, JSON.stringify(coverage, null, 2) + '\n')
  console.log(`recorded: model compared with the covered code at ${head.slice(0, 7)} plus the working tree`)
  process.exit(0)
}

console.log('SOS delivery: exhaustive check of every order of steps')
console.log(`bounds: 1 delivery, ${bounds.workers.length} workers, gives up at attempt ${bounds.maxAttempts} (code: ${facts.sosMaxAttempts})`)
console.log(
  `timing premise from worker.go: leaseFor ${facts.leaseFor}s > sendTimeout ${facts.sendTimeout}s + dbTimeout ${facts.dbTimeout}s: ${facts.leaseOutlivesWork ? 'holds' : 'BROKEN - a live worker can outlive its lease'}`
)
console.log('')

let mismatches = 0
for (const sc of scenarios) {
  for (const p of sc.props) {
    const r = check(sc.cfg, p)
    const got = r.ok ? 'hold' : 'break'
    const asExpected = got === sc.expect
    if (!asExpected) mismatches++
    const verdict = r.ok ? `holds in all ${r.states} states` : `BROKEN after ${r.trace.length} steps`
    console.log(`${asExpected ? 'as expected' : 'UNEXPECTED '}  ${sc.name.padEnd(13)} ${p.padEnd(17)} ${verdict}   [${sc.what}]`)
    // Show a trace when an outcome is a surprise.
    const show = !r.ok && !asExpected
    if (!asExpected && r.ok) console.log(`      expected this to break: the property can no longer fail here, so it no longer guards "${sc.what}"`)
    if (show) {
      console.log(`      promise: ${properties[p].promise}`)
      console.log(`      harm:    ${properties[p].harm}`)
      r.trace.forEach((l, i) => console.log(`      ${String(i + 1).padStart(2)}. ${l}`))
      const e = r.end
      console.log(`      ends with: status=${e.status} attempts=${e.attempts} everSent=${e.everSent} opsAlert=${e.opsAlert}`)
    }
  }
}

const changed = drift()
console.log('')
if (changed.length) {
  console.log(`DRIFT: covered code changed since the model was last compared with it (${coverage.syncCommit.slice(0, 7)}):`)
  for (const f of changed) console.log(`  ${f}`)
  console.log('  Compare the change with model.mjs (SKILL.md, "When the covered code changed"), then run with --accept.')
}
if (mismatches) {
  console.log(`RESULT: ${mismatches} outcome(s) differ from the expectation. Read the trace above.`)
  process.exit(1)
}
if (changed.length) {
  console.log('RESULT: outcomes are as expected, but they describe the code as it was. Not a pass until the drift is reviewed.')
  process.exit(2)
}
console.log('RESULT: as expected. All four properties hold on the code as modelled.')
