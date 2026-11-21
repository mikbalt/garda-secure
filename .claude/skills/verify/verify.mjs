#!/usr/bin/env node
/**
 * GARDA verification driver (web dashboard, demo build under the production CSP).
 *
 *   node .claude/skills/verify/verify.mjs launch  [--port 4273]
 *   node .claude/skills/verify/verify.mjs doctor  [--port 4273]
 *   node .claude/skills/verify/verify.mjs drive <journey.mjs> [--port 4273] [--name label] [--headed] [--mobile]
 *   node .claude/skills/verify/verify.mjs cleanup [--port 4273]
 *
 * Instance state:  .claude/verify/instance-<port>/   (dist, pid.json, server.log) - removed by cleanup
 * Evidence:        .claude/verify/evidence/<stamp>-<name>/                        - never removed
 */
import { spawn, spawnSync } from 'node:child_process'
import fs from 'node:fs'
import { createRequire } from 'node:module'
import net from 'node:net'
import path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

const HERE = path.dirname(fileURLToPath(import.meta.url))
const REPO = path.resolve(HERE, '..', '..', '..')
const WEB = path.join(REPO, 'web')
const STATE = path.join(REPO, '.claude', 'verify')
const VITE = path.join(WEB, 'node_modules', 'vite', 'bin', 'vite.js')

const argv = process.argv.slice(2)
const cmd = argv[0]
function flag(name, fallback) {
  const i = argv.indexOf(`--${name}`)
  if (i === -1) return fallback
  const next = argv[i + 1]
  return next && !next.startsWith('--') ? next : true
}
const PORT = Number(flag('port', process.env.VERIFY_PORT ?? 4273))
const BASE = `http://127.0.0.1:${PORT}`
const INSTANCE = path.join(STATE, `instance-${PORT}`)
const DIST = path.join(INSTANCE, 'dist')
const PID_FILE = path.join(INSTANCE, 'pid.json')
const LOG_FILE = path.join(INSTANCE, 'server.log')

function die(msg) {
  console.error(`FAIL: ${msg}`)
  process.exit(1)
}
function git(...args) {
  const r = spawnSync('git', args, { cwd: REPO, encoding: 'utf8' })
  return r.status === 0 ? r.stdout.trim() : ''
}
function alive(pid) {
  try {
    process.kill(pid, 0)
    return true
  } catch {
    return false
  }
}
function portOpen() {
  return new Promise((resolve) => {
    const s = net.connect({ host: '127.0.0.1', port: PORT })
    s.once('connect', () => (s.destroy(), resolve(true)))
    s.once('error', () => resolve(false))
    s.setTimeout(1500, () => (s.destroy(), resolve(false)))
  })
}
function readPid() {
  try {
    return JSON.parse(fs.readFileSync(PID_FILE, 'utf8'))
  } catch {
    return null
  }
}
/** Newest mtime (ms) of the files that go into the demo build. */
function newestSource() {
  let newest = 0
  const walk = (p) => {
    const st = fs.statSync(p)
    if (st.isDirectory()) for (const e of fs.readdirSync(p)) walk(path.join(p, e))
    else if (st.mtimeMs > newest) newest = st.mtimeMs
  }
  for (const p of ['src', 'public', 'index.html', 'vite.config.ts', '.env.demo', 'package-lock.json']) {
    const full = path.join(WEB, p)
    if (fs.existsSync(full)) walk(full)
  }
  return newest
}
/** The instance serves the GARDA app shell, its entry script and the production CSP. */
async function probe() {
  const res = await fetch(BASE + '/', { signal: AbortSignal.timeout(5000) })
  const html = await res.text()
  const script = /<script[^>]+src="([^"]+)"/.exec(html)?.[1]
  const out = {
    status: res.status,
    root: html.includes('id="root"'),
    csp: (res.headers.get('content-security-policy') ?? '').includes("script-src 'self'"),
    entry: false,
  }
  if (script) out.entry = (await fetch(new URL(script, BASE), { signal: AbortSignal.timeout(5000) })).ok
  return out
}
const probeOk = (p) => p.status === 200 && p.root && p.csp && p.entry

async function launch() {
  if (!fs.existsSync(VITE)) die('web/node_modules is missing: run `npm ci` in web/ first')
  const old = readPid()
  if (old && alive(old.pid)) die(`instance already running on port ${PORT} (pid ${old.pid}); run doctor, or cleanup first`)
  if (await portOpen()) die(`port ${PORT} is used by a process this skill did not start; pick another with --port`)
  fs.rmSync(INSTANCE, { recursive: true, force: true })
  fs.mkdirSync(INSTANCE, { recursive: true })

  console.log(`building the demo bundle into ${path.relative(REPO, DIST)} ...`)
  const build = spawnSync(process.execPath, [VITE, 'build', '--mode', 'demo', '--outDir', DIST, '--emptyOutDir'], {
    cwd: WEB,
    encoding: 'utf8',
  })
  fs.writeFileSync(path.join(INSTANCE, 'build.log'), `${build.stdout}\n${build.stderr}`)
  if (build.status !== 0) die(`vite build failed, see ${path.relative(REPO, path.join(INSTANCE, 'build.log'))}\n${build.stderr}`)

  const log = fs.openSync(LOG_FILE, 'a')
  const child = spawn(
    process.execPath,
    [VITE, 'preview', '--mode', 'demo', '--port', String(PORT), '--strictPort', '--host', '127.0.0.1', '--outDir', DIST],
    { cwd: WEB, detached: true, stdio: ['ignore', log, log], windowsHide: true }
  )
  child.unref()
  fs.writeFileSync(
    PID_FILE,
    JSON.stringify({ pid: child.pid, port: PORT, builtAt: Date.now(), head: git('rev-parse', 'HEAD'), dirty: git('status', '--porcelain', 'web') !== '' }, null, 2)
  )

  const deadline = Date.now() + 30_000
  while (Date.now() < deadline) {
    if (!alive(child.pid)) die(`preview server exited, see ${path.relative(REPO, LOG_FILE)}`)
    try {
      if (probeOk(await probe())) {
        console.log(`READY ${BASE} (pid ${child.pid})`)
        return
      }
    } catch {
      /* not listening yet */
    }
    await new Promise((r) => setTimeout(r, 300))
  }
  die(`preview server did not become ready in 30 s, see ${path.relative(REPO, LOG_FILE)}`)
}

async function doctor({ quiet = false } = {}) {
  const checks = []
  const add = (name, ok, detail = '') => checks.push({ name, ok, detail })
  const info = readPid()
  add('instance was started by this skill', !!info, info ? `pid ${info.pid}` : `no ${path.relative(REPO, PID_FILE)}; run launch`)
  if (info) {
    add('process is alive', alive(info.pid))
    let p = null
    try {
      p = await probe()
    } catch (e) {
      add(`${BASE} answers`, false, String(e.message ?? e))
    }
    if (p) {
      add(`${BASE} answers 200 with the app shell`, p.status === 200 && p.root, `status ${p.status}`)
      add('entry script loads', p.entry)
      add('production CSP header is served', p.csp)
    }
    const head = git('rev-parse', 'HEAD')
    add('build is from the current commit', info.head === head, `built ${info.head.slice(0, 7)}, HEAD ${head.slice(0, 7)}`)
    add('no web source changed since the build', newestSource() <= info.builtAt, 'relaunch (cleanup + launch) after editing web/')
  }
  const ok = checks.every((c) => c.ok)
  if (!quiet || !ok) for (const c of checks) console.log(`${c.ok ? 'ok  ' : 'FAIL'} ${c.name}${c.ok || !c.detail ? '' : ` - ${c.detail}`}`)
  if (!quiet) console.log(ok ? 'DOCTOR OK' : 'DOCTOR FAILED')
  return ok
}

async function drive() {
  const journeyArg = argv[1]
  if (!journeyArg || journeyArg.startsWith('--')) die('usage: drive <journey.mjs> [--name label] [--headed] [--mobile]')
  const journeyPath = path.resolve(journeyArg)
  if (!fs.existsSync(journeyPath)) die(`journey not found: ${journeyPath}`)
  if (!(await doctor({ quiet: true }))) die('instance is not worth driving (see above)')

  const req = createRequire(path.join(WEB, 'package.json'))
  const { chromium, expect } = req('@playwright/test')
  const name = String(flag('name', path.basename(journeyPath, '.mjs')))
  const stamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19)
  const outDir = path.join(STATE, 'evidence', `${stamp}-${name}`)
  fs.mkdirSync(outDir, { recursive: true })

  const headless = !flag('headed', false)
  let browser
  const channels = [process.env.PW_CHANNEL || 'chromium', 'chrome']
  for (const channel of channels) {
    try {
      browser = await chromium.launch({ channel, headless })
      break
    } catch (e) {
      if (channel === channels.at(-1)) die(`no browser: ${e.message.split('\n')[0]} (run \`npx playwright install chromium\` in web/ or install Chrome)`)
    }
  }
  const mobile = !!flag('mobile', false)
  const context = await browser.newContext({
    baseURL: BASE,
    locale: 'id-ID',
    timezoneId: 'Asia/Jakarta',
    viewport: mobile ? { width: 390, height: 844 } : { width: 1280, height: 900 },
    hasTouch: mobile,
    acceptDownloads: true,
  })
  await context.tracing.start({ screenshots: true, snapshots: true })
  const page = await context.newPage()
  expect.configure?.({ timeout: 10_000 })
  page.setDefaultTimeout(15_000)

  const problems = []
  page.on('console', (m) => m.type() === 'error' && problems.push(`console: ${m.text()}`))
  page.on('pageerror', (e) => problems.push(`pageerror: ${e.message}`))
  await page.addInitScript(() => {
    document.addEventListener('securitypolicyviolation', (e) => console.error(`CSP violation: ${e.violatedDirective} ${e.blockedURI}`))
  })

  const steps = []
  let n = 0
  const log = (kind, text, extra = {}) => {
    steps.push({ at: new Date().toISOString(), kind, text, url: page.url(), ...extra })
    console.log(`  [${kind}] ${text}`)
  }
  const quiet = (p = page) =>
    p.waitForFunction(
      () => document.getAnimations().every((a) => a.playState !== 'running' || a.effect?.getComputedTiming().iterations === Infinity),
      undefined,
      { timeout: 5000 }
    )
  const settle = async () => {
    await expect(page.locator('main h1').first()).toBeVisible()
    await expect(page.locator('main .animate-pulse')).toHaveCount(0, { timeout: 15_000 })
    await quiet()
  }
  const api = {
    page,
    context,
    expect,
    baseURL: BASE,
    evidenceDir: outDir,
    settle,
    quiet,
    /** Record a fact the journey observed (shows up in result.json). */
    note: (text) => log('note', text),
    /** Screenshot after an action; numbered so the sequence reads as before/after. */
    shot: async (label) => {
      const file = `${String(++n).padStart(2, '0')}-${label.replace(/[^a-z0-9]+/gi, '-').toLowerCase()}.png`
      await page.screenshot({ path: path.join(outDir, file) })
      log('shot', label, { file })
    },
    /** Open the demo app as a role: admin | supervisor | hr | client-readonly | platform-admin. */
    openAs: async (role, urlPath = '/', lang = 'id') => {
      await page.addInitScript(
        ([r, l]) => {
          try {
            localStorage.setItem('sp.demo.role', r)
            localStorage.setItem('sp.lang', l)
            localStorage.setItem('sp.sos.muted', '1')
          } catch {
            /* ignore */
          }
        },
        [role, lang]
      )
      await page.goto(urlPath)
      await settle()
      log('open', `${role} ${urlPath}`)
    },
    /** Navigate inside the same session (the demo data survives; a new drive starts from the seed). */
    goto: async (urlPath) => {
      await page.goto(urlPath)
      await settle()
      log('goto', urlPath)
    },
    /** The demo backend's data (sessionStorage `sp.demo.world`): the side-effect check for writes. */
    world: () => page.evaluate(() => JSON.parse(sessionStorage.getItem('sp.demo.world') ?? 'null')),
    /** Save a download triggered by `action` into the evidence dir; returns its file name. */
    download: async (action) => {
      const [dl] = await Promise.all([page.waitForEvent('download'), action()])
      const file = dl.suggestedFilename()
      await dl.saveAs(path.join(outDir, file))
      log('download', file, { file, bytes: fs.statSync(path.join(outDir, file)).size })
      return file
    },
  }

  let error = null
  console.log(`driving ${path.relative(REPO, journeyPath)} against ${BASE}`)
  try {
    const mod = await import(pathToFileURL(journeyPath).href)
    if (typeof mod.default !== 'function') throw new Error('journey must `export default async function (t) {}`')
    await mod.default(api)
  } catch (e) {
    error = String(e?.stack ?? e)
    await page.screenshot({ path: path.join(outDir, 'FAILED.png') }).catch(() => {})
  }
  const world = await api.world().catch(() => null)
  if (world) fs.writeFileSync(path.join(outDir, 'world-after.json'), JSON.stringify(world, null, 2))
  await context.tracing.stop({ path: path.join(outDir, 'trace.zip') }).catch(() => {})
  await browser.close()

  const ok = !error && problems.length === 0
  const info = readPid()
  fs.writeFileSync(
    path.join(outDir, 'result.json'),
    JSON.stringify(
      { ok, journey: path.relative(REPO, journeyPath), stack: 'web demo build (in-browser mock API), production CSP', baseURL: BASE, head: info?.head, dirtyAtBuild: info?.dirty, error, problems, steps },
      null,
      2
    )
  )
  if (error) console.error(error)
  for (const p of problems) console.error(`problem: ${p}`)
  console.log(`${ok ? 'PASS' : 'FAIL'} evidence: ${path.relative(REPO, outDir)}`)
  process.exit(ok ? 0 : 1)
}

async function cleanup() {
  const info = readPid()
  if (info && alive(info.pid)) {
    // Only kill what launch started: the recorded pid must still own our port.
    if (await portOpen()) {
      process.kill(info.pid)
      const deadline = Date.now() + 5000
      while (alive(info.pid) && Date.now() < deadline) await new Promise((r) => setTimeout(r, 100))
      console.log(`stopped preview server pid ${info.pid}`)
    } else {
      console.log(`pid ${info.pid} is alive but port ${PORT} is closed: not ours any more, left alone`)
    }
  }
  // Windows may hold the log for a moment after the process exits.
  for (let i = 0; i < 10 && fs.existsSync(INSTANCE); i++) {
    try {
      fs.rmSync(INSTANCE, { recursive: true, force: true })
    } catch {
      await new Promise((r) => setTimeout(r, 300))
    }
  }
  if (fs.existsSync(INSTANCE)) die(`could not remove ${path.relative(REPO, INSTANCE)}`)
  const ev = path.join(STATE, 'evidence')
  console.log(`CLEAN (evidence kept in ${path.relative(REPO, ev)}: ${fs.existsSync(ev) ? fs.readdirSync(ev).length : 0} run(s))`)
}

const commands = { launch, doctor: async () => process.exit((await doctor()) ? 0 : 1), drive, cleanup }
if (!commands[cmd]) die('usage: verify.mjs launch|doctor|drive <journey.mjs>|cleanup [--port N]')
await commands[cmd]()
