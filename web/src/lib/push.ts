/**
 * Web push for the dashboard: browser detection, the state of "this device",
 * and the enable / disable / account-switch flows (service worker
 * `public/sw.js`, API `/push/*`). Pure helpers are exported for tests; the
 * flows take the API as a parameter so tests can fake it.
 */
import apiClient, { apiErrorStatus, apiGet } from '../api/client'
import { config } from '../config'
import type { CreatePushSubscription, PushConfig, PushSubscriptionRow } from '../types/push'

export const SW_URL = '/sw.js'
export const SW_SCOPE = '/'

/**
 * State of push on this browser:
 * - `unsupported`: no service worker / Push API / Notification API (or iOS < 16.4)
 * - `ios-needs-install`: iPhone/iPad Safari tab — push only works from the Home Screen app
 * - `server-disabled`: the server has no web push configured (`webpush.enabled` false)
 * - `blocked`: the user denied notifications for this site
 * - `off` / `on`: `on` = this browser has a subscription the server lists for the user
 * - `loading` / `error`: config or subscription list not (yet) available
 */
export type PushState = 'loading' | 'error' | 'unsupported' | 'ios-needs-install' | 'server-disabled' | 'blocked' | 'off' | 'on'

export interface PushEnv {
  /** Service worker + PushManager + Notification available. */
  supported: boolean
  ios: boolean
  /** iOS version as major*100+minor (1604 = 16.4), when known. */
  iosVersion?: number
  /** Running as an installed app (Home Screen / PWA window). */
  standalone: boolean
}

export type PushErrorCode = 'unsupported' | 'denied' | 'dismissed' | 'no_worker' | 'subscribe_failed' | 'invalid_key'

/** A browser-side failure of the push flows (API errors are thrown as they are). */
export class PushError extends Error {
  constructor(public readonly code: PushErrorCode) {
    super(`push: ${code}`)
    this.name = 'PushError'
  }
}

// ── detection ────────────────────────────────────────────────────────────────

export function isIOS(ua: string, platform = '', maxTouchPoints = 0): boolean {
  // iPadOS 13+ Safari reports a Mac user agent; touch support gives it away.
  return /iPhone|iPad|iPod/i.test(ua) || (platform === 'MacIntel' && maxTouchPoints > 1)
}

/** iOS version from the user agent (1604 = 16.4), or undefined. */
export function iosVersion(ua: string): number | undefined {
  const m = /(?:iPhone|iPad|iPod|CPU)(?: iPhone)? OS (\d+)_(\d+)/.exec(ua) ?? (/Macintosh/.test(ua) ? /Version\/(\d+)\.(\d+)/.exec(ua) : null)
  return m ? Number(m[1]) * 100 + Number(m[2]) : undefined
}

export function pushSupported(w: Window | undefined = typeof window === 'undefined' ? undefined : window): boolean {
  return !!w && 'serviceWorker' in w.navigator && 'PushManager' in w && 'Notification' in w
}

export function isStandalone(w: Window = window): boolean {
  if ((w.navigator as Navigator & { standalone?: boolean }).standalone === true) return true
  try {
    return !!w.matchMedia?.('(display-mode: standalone)').matches
  } catch {
    return false
  }
}

export function detectEnv(w: Window = window): PushEnv {
  const ua = w.navigator.userAgent
  const ios = isIOS(ua, w.navigator.platform, w.navigator.maxTouchPoints)
  return { supported: pushSupported(w), ios, iosVersion: ios ? iosVersion(ua) : undefined, standalone: isStandalone(w) }
}

/** Human label of this browser for the device list, e.g. "Chrome on Windows". */
export function deviceLabel(ua: string, platform = '', maxTouchPoints = 0): string {
  const browser = /EdgiOS|Edg(e|A)?\//.test(ua)
    ? 'Edge'
    : /OPR\/|Opera/.test(ua)
      ? 'Opera'
      : /SamsungBrowser/.test(ua)
        ? 'Samsung Internet'
        : /Firefox|FxiOS/.test(ua)
          ? 'Firefox'
          : /CriOS|Chrome\//.test(ua)
            ? 'Chrome'
            : /Safari\//.test(ua)
              ? 'Safari'
              : 'Browser'
  const os = /iPhone|iPod/.test(ua)
    ? 'iPhone'
    : /iPad/.test(ua) || (platform === 'MacIntel' && maxTouchPoints > 1)
      ? 'iPad'
      : /Android/.test(ua)
        ? 'Android'
        : /Windows/.test(ua)
          ? 'Windows'
          : /CrOS/.test(ua)
            ? 'ChromeOS'
            : /Mac OS X|Macintosh/.test(ua)
              ? 'macOS'
              : /Linux/.test(ua)
                ? 'Linux'
                : ''
  return os ? `${browser} on ${os}` : browser
}

export function thisDeviceLabel(): string {
  return deviceLabel(navigator.userAgent, navigator.platform, navigator.maxTouchPoints)
}

// ── keys ─────────────────────────────────────────────────────────────────────

/** Decodes a base64url VAPID public key (applicationServerKey). */
export function urlBase64ToUint8Array(value: string): Uint8Array<ArrayBuffer> {
  const s = value.trim()
  const b64 = (s + '='.repeat((4 - (s.length % 4)) % 4)).replace(/-/g, '+').replace(/_/g, '/')
  let raw: string
  try {
    raw = atob(b64)
  } catch {
    throw new PushError('invalid_key')
  }
  const out = new Uint8Array(raw.length)
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i)
  return out
}

export function sameBytes(a: ArrayBuffer | ArrayBufferView | null | undefined, b: Uint8Array): boolean {
  if (!a) return false
  const x = a instanceof ArrayBuffer ? new Uint8Array(a) : new Uint8Array(a.buffer, a.byteOffset, a.byteLength)
  return x.length === b.length && x.every((v, i) => v === b[i])
}

/** Whether a subscription was made with the server's current key (unknown key = assume yes). */
export function keyMatches(subKey: ArrayBuffer | null | undefined, vapidKey: string | undefined): boolean {
  if (!subKey || !vapidKey) return true
  try {
    return sameBytes(subKey, urlBase64ToUint8Array(vapidKey))
  } catch {
    return true
  }
}

// ── state ────────────────────────────────────────────────────────────────────

export interface StateInput {
  env: PushEnv
  permission: NotificationPermission | 'unsupported'
  /** undefined while loading */
  config?: PushConfig
  /** config or subscription list failed */
  error?: boolean
  /** this browser's endpoint; undefined while loading, null = none */
  browserEndpoint?: string | null
  /** endpoints the server lists for the user; undefined while loading */
  serverEndpoints?: string[]
  /** the browser subscription uses the server's current key */
  keyMatches?: boolean
}

export function derivePushState(i: StateInput): PushState {
  const { env } = i
  if (env.ios && !env.standalone) return env.iosVersion !== undefined && env.iosVersion < 1604 ? 'unsupported' : 'ios-needs-install'
  if (!env.supported) return 'unsupported'
  if (i.error) return 'error'
  if (!i.config) return 'loading'
  if (!i.config.webpush?.enabled || !i.config.webpush.vapid_public_key) return 'server-disabled'
  if (i.permission === 'denied') return 'blocked'
  if (i.browserEndpoint === undefined || i.serverEndpoints === undefined) return 'loading'
  if (i.permission === 'granted' && i.browserEndpoint && i.keyMatches !== false && i.serverEndpoints.includes(i.browserEndpoint)) return 'on'
  return 'off'
}

/** Only same-origin app paths ("/incidents?tab=sos"); anything else opens the start page. */
export function safeAppPath(url: unknown): string {
  return typeof url === 'string' && /^\/(?![/\\])/.test(url) ? url : '/'
}

// ── API ──────────────────────────────────────────────────────────────────────

export interface PushApi {
  create(body: CreatePushSubscription): Promise<{ id: string }>
  remove(id: string): Promise<void>
  list(): Promise<PushSubscriptionRow[]>
}

export const pushApi: PushApi = {
  create: async (body) => (await apiClient.post<{ id: string }>('/push/subscriptions', body)).data,
  remove: async (id) => {
    await apiClient.delete(`/push/subscriptions/${encodeURIComponent(id)}`)
  },
  list: async () => {
    const rows = await apiGet<unknown>('/push/subscriptions')
    return Array.isArray(rows) ? (rows as PushSubscriptionRow[]) : []
  },
}

export const fetchPushConfig = () => apiGet<PushConfig>('/push/config')

// ── browser side ─────────────────────────────────────────────────────────────

/**
 * Demo mode only: a stand-in subscription when the browser cannot reach a
 * real push service, so the page stays usable. Lives until the next reload,
 * like the demo server's data.
 */
let simulated: { endpoint: string } | null = null

export function simulatedSubscription(): { endpoint: string } | null {
  return simulated
}

/** Test hook. */
export function resetPushForTests(): void {
  simulated = null
}

function withTimeout<T>(p: Promise<T>, ms: number, onTimeout: () => Error): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const id = setTimeout(() => reject(onTimeout()), ms)
    p.then(
      (v) => (clearTimeout(id), resolve(v)),
      (e) => (clearTimeout(id), reject(e))
    )
  })
}

/** Registers the push service worker (silently does nothing where unsupported). */
export async function registerServiceWorker(): Promise<ServiceWorkerRegistration | undefined> {
  if (!pushSupported()) return undefined
  try {
    return await navigator.serviceWorker.register(SW_URL, { scope: SW_SCOPE })
  } catch {
    return undefined
  }
}

async function activeRegistration(timeoutMs: number): Promise<ServiceWorkerRegistration> {
  let reg: ServiceWorkerRegistration | undefined
  try {
    reg = (await navigator.serviceWorker.getRegistration(SW_SCOPE)) ?? (await navigator.serviceWorker.register(SW_URL, { scope: SW_SCOPE }))
  } catch {
    throw new PushError('no_worker')
  }
  if (reg.active) return reg
  return withTimeout(navigator.serviceWorker.ready, timeoutMs, () => new PushError('no_worker'))
}

async function browserSubscription(): Promise<PushSubscription | null> {
  if (!pushSupported()) return null
  try {
    const reg = await navigator.serviceWorker.getRegistration(SW_SCOPE)
    return (await reg?.pushManager.getSubscription()) ?? null
  } catch {
    return null
  }
}

export interface BrowserPushInfo {
  endpoint: string | null
  /** applicationServerKey the subscription was made with */
  key: ArrayBuffer | null
  simulated: boolean
}

/** This browser's push subscription (demo: the simulated one). */
export async function browserPushInfo(): Promise<BrowserPushInfo> {
  const sub = await browserSubscription()
  if (sub) return { endpoint: sub.endpoint, key: sub.options?.applicationServerKey ?? null, simulated: false }
  return { endpoint: simulated?.endpoint ?? null, key: null, simulated: !!simulated }
}

async function askPermission(): Promise<NotificationPermission> {
  try {
    return await Notification.requestPermission()
  } catch {
    return Notification.permission
  }
}

function randomId(): string {
  try {
    return crypto.randomUUID()
  } catch {
    return Math.random().toString(36).slice(2) + Date.now().toString(36)
  }
}

export interface EnableOptions {
  vapidKey: string
  label: string
  /** The user's rows on the server: a row of a replaced browser subscription is removed. */
  rows?: PushSubscriptionRow[]
  api?: PushApi
  demo?: boolean
  /** How long to wait for the service worker to activate. */
  timeoutMs?: number
}

export interface EnableResult {
  id: string
  endpoint: string
  simulated: boolean
}

/**
 * Turns push on for this browser. Call it from a click handler: the
 * permission prompt needs the user gesture. Any existing subscription
 * (rotated server key, another account's, or one the server forgot) is
 * replaced by a fresh one, so an endpoint never serves two accounts.
 */
export async function enablePush(opts: EnableOptions): Promise<EnableResult> {
  const api = opts.api ?? pushApi
  const demo = opts.demo ?? config.demo
  if (!pushSupported()) throw new PushError('unsupported')
  const perm = await askPermission()
  if (perm === 'denied') throw new PushError('denied')
  if (perm !== 'granted') throw new PushError('dismissed')
  const key = urlBase64ToUint8Array(opts.vapidKey)

  let sub: PushSubscription
  try {
    const reg = await activeRegistration(opts.timeoutMs ?? 10_000)
    const old = await reg.pushManager.getSubscription()
    if (old) {
      const stale = opts.rows?.find((r) => r.endpoint === old.endpoint)
      if (stale) await api.remove(stale.id).catch(() => undefined)
      await old.unsubscribe().catch(() => false)
    }
    sub = await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: key })
  } catch (e) {
    if (demo) return simulate(api, opts.label)
    throw e instanceof PushError ? e : new PushError('subscribe_failed')
  }

  const json = sub.toJSON()
  const p256dh = json.keys?.p256dh
  const auth = json.keys?.auth
  const endpoint = json.endpoint ?? sub.endpoint
  if (!endpoint || !p256dh || !auth) {
    await sub.unsubscribe().catch(() => false)
    throw new PushError('subscribe_failed')
  }
  try {
    const { id } = await api.create({ kind: 'webpush', endpoint, keys: { p256dh, auth }, label: opts.label })
    simulated = null
    return { id, endpoint, simulated: false }
  } catch (e) {
    // The server did not take it: do not leave an orphan subscription behind.
    await sub.unsubscribe().catch(() => false)
    throw e
  }
}

async function simulate(api: PushApi, label: string): Promise<EnableResult> {
  const endpoint = `https://demo.push.invalid/${randomId()}`
  const { id } = await api.create({ kind: 'webpush', endpoint, keys: { p256dh: 'demo-p256dh', auth: 'demo-auth' }, label })
  simulated = { endpoint }
  return { id, endpoint, simulated: true }
}

/** Turns push off for this browser: removes the server row, then unsubscribes. */
export async function disablePush(opts: { rows: PushSubscriptionRow[]; api?: PushApi }): Promise<void> {
  const api = opts.api ?? pushApi
  const sub = await browserSubscription()
  const endpoint = sub?.endpoint ?? simulated?.endpoint ?? null
  const row = endpoint ? opts.rows.find((r) => r.endpoint === endpoint) : undefined
  if (row) {
    try {
      await api.remove(row.id)
    } catch (e) {
      if (apiErrorStatus(e) !== 404) throw e
    }
  }
  if (sub) await sub.unsubscribe().catch(() => false)
  simulated = null
}

/**
 * Before switching organisation / account: stop this browser from receiving
 * the current user's notifications. Best effort and bounded in time — the
 * switch must never hang on it. `server: false` when no API token is
 * available (sign-in recovery screens); the server then drops the row on the
 * next failed delivery.
 */
export async function dropPushForAccountSwitch({ server = true, api = pushApi, timeoutMs = 3_000 }: { server?: boolean; api?: PushApi; timeoutMs?: number } = {}): Promise<void> {
  const work = (async () => {
    const sub = await browserSubscription()
    const endpoint = sub?.endpoint ?? simulated?.endpoint
    simulated = null
    if (!endpoint) return
    const local = sub ? sub.unsubscribe().catch(() => false) : Promise.resolve(true)
    const remote = server
      ? api
          .list()
          .then((rows) => rows.find((r) => r.endpoint === endpoint))
          .then((row) => (row ? api.remove(row.id) : undefined))
          .catch(() => undefined)
      : Promise.resolve()
    await Promise.all([local, remote])
  })()
  await Promise.race([work.catch(() => undefined), new Promise((r) => setTimeout(r, timeoutMs))])
}

/** Demo mode: shows the test notification locally (no real push service delivers it). */
export async function showLocalTestNotification(title: string, body: string): Promise<void> {
  if (!pushSupported() || Notification.permission !== 'granted') return
  try {
    const reg = await navigator.serviceWorker.getRegistration(SW_SCOPE)
    await reg?.showNotification(title, { body, tag: 'garda-test', icon: '/icons/icon-192.png', badge: '/icons/favicon-32.png', data: { url: '/notifications' } })
  } catch {
    /* ignore */
  }
}
