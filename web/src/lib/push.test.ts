import { AxiosError, AxiosHeaders, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  PushError,
  browserPushInfo,
  derivePushState,
  deviceLabel,
  disablePush,
  dropPushForAccountSwitch,
  enablePush,
  iosVersion,
  isIOS,
  keyMatches,
  resetPushForTests,
  safeAppPath,
  urlBase64ToUint8Array,
  type PushApi,
  type PushEnv,
  type StateInput,
} from './push'
import type { PushConfig, PushSubscriptionRow } from '../types/push'

const KEY = 'BMq1fmPg8AzlxBtu9BAMXXrnv7ZHddAm_OP_dwQGJNHzfDtLx_jfbPnTcbGpLln9Z9ffQpmSLrj8BM_Jdh5uM3c'
const OTHER_KEY = 'BEl62iUYgUivxIkv69yViEuiBIa-Ib9-SkvMeAtA3LFgDzkrxZJjSgSnfckjBJuBkr3qBUYIHBQFLXYp5Nksh8U'

const UA = {
  chromeWin: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36',
  edgeWin: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36 Edg/129.0.0.0',
  firefoxLinux: 'Mozilla/5.0 (X11; Linux x86_64; rv:131.0) Gecko/20100101 Firefox/131.0',
  chromeAndroid: 'Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Mobile Safari/537.36',
  samsung: 'Mozilla/5.0 (Linux; Android 14; SM-S918B) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/26.0 Chrome/122.0.0.0 Mobile Safari/537.36',
  safariIphone: 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1',
  oldIphone: 'Mozilla/5.0 (iPhone; CPU iPhone OS 15_7 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/15.6 Mobile/15E148 Safari/604.1',
  ipadDesktop: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15',
  safariMac: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15',
}

describe('urlBase64ToUint8Array', () => {
  it('decodes an unpadded base64url P-256 public key (65 bytes, uncompressed)', () => {
    const k = urlBase64ToUint8Array(KEY)
    expect(k).toBeInstanceOf(Uint8Array)
    expect(k.length).toBe(65)
    expect(k[0]).toBe(4)
  })

  it('maps - and _ and pads', () => {
    expect(Array.from(urlBase64ToUint8Array('-_8'))).toEqual([0xfb, 0xff])
    expect(Array.from(urlBase64ToUint8Array(' AQID '))).toEqual([1, 2, 3])
  })

  it('rejects garbage with invalid_key', () => {
    expect(() => urlBase64ToUint8Array('***')).toThrow(PushError)
    try {
      urlBase64ToUint8Array('***')
    } catch (e) {
      expect((e as PushError).code).toBe('invalid_key')
    }
  })

  it('keyMatches compares the subscription key with the server key', () => {
    expect(keyMatches(urlBase64ToUint8Array(KEY).buffer, KEY)).toBe(true)
    expect(keyMatches(urlBase64ToUint8Array(OTHER_KEY).buffer, KEY)).toBe(false)
    expect(keyMatches(null, KEY)).toBe(true)
  })
})

describe('device label and iOS detection', () => {
  it('names browser and OS', () => {
    expect(deviceLabel(UA.chromeWin)).toBe('Chrome on Windows')
    expect(deviceLabel(UA.edgeWin)).toBe('Edge on Windows')
    expect(deviceLabel(UA.firefoxLinux)).toBe('Firefox on Linux')
    expect(deviceLabel(UA.chromeAndroid)).toBe('Chrome on Android')
    expect(deviceLabel(UA.samsung)).toBe('Samsung Internet on Android')
    expect(deviceLabel(UA.safariIphone)).toBe('Safari on iPhone')
    expect(deviceLabel(UA.ipadDesktop, 'MacIntel', 5)).toBe('Safari on iPad')
    expect(deviceLabel(UA.safariMac, 'MacIntel', 0)).toBe('Safari on macOS')
    expect(deviceLabel('curl/8')).toBe('Browser')
  })

  it('detects iPhone and iPadOS (desktop user agent with touch)', () => {
    expect(isIOS(UA.safariIphone)).toBe(true)
    expect(isIOS(UA.ipadDesktop, 'MacIntel', 5)).toBe(true)
    expect(isIOS(UA.safariMac, 'MacIntel', 0)).toBe(false)
    expect(isIOS(UA.chromeAndroid, 'Linux armv8l', 5)).toBe(false)
  })

  it('reads the iOS version', () => {
    expect(iosVersion(UA.safariIphone)).toBe(1705)
    expect(iosVersion(UA.oldIphone)).toBe(1507)
    expect(iosVersion(UA.ipadDesktop)).toBe(1704)
  })
})

describe('derivePushState', () => {
  const desktop: PushEnv = { supported: true, ios: false, standalone: false }
  const cfg: PushConfig = { webpush: { enabled: true, vapid_public_key: KEY }, fcm: { enabled: true } }
  const base: StateInput = { env: desktop, permission: 'granted', config: cfg, browserEndpoint: 'https://e/1', serverEndpoints: ['https://e/1'], keyMatches: true }
  const s = (over: Partial<StateInput>) => derivePushState({ ...base, ...over })

  it('is on when the server lists this browser and permission is granted', () => {
    expect(s({})).toBe('on')
  })

  it('is off without a subscription, when the server does not know it, or the key rotated', () => {
    expect(s({ browserEndpoint: null })).toBe('off')
    expect(s({ serverEndpoints: [] })).toBe('off')
    expect(s({ keyMatches: false })).toBe('off')
    expect(s({ permission: 'default' })).toBe('off')
  })

  it('reports blocked, server-disabled, unsupported, loading and error', () => {
    expect(s({ permission: 'denied' })).toBe('blocked')
    expect(s({ config: { webpush: { enabled: false, vapid_public_key: '' }, fcm: { enabled: false } } })).toBe('server-disabled')
    expect(s({ env: { supported: false, ios: false, standalone: false }, permission: 'unsupported' })).toBe('unsupported')
    expect(s({ config: undefined })).toBe('loading')
    expect(s({ browserEndpoint: undefined })).toBe('loading')
    expect(s({ error: true })).toBe('error')
  })

  it('asks iOS users to install first; old iOS is unsupported', () => {
    expect(s({ env: { supported: false, ios: true, iosVersion: 1705, standalone: false } })).toBe('ios-needs-install')
    expect(s({ env: { supported: false, ios: true, standalone: false } })).toBe('ios-needs-install')
    expect(s({ env: { supported: false, ios: true, iosVersion: 1507, standalone: false } })).toBe('unsupported')
    expect(s({ env: { supported: true, ios: true, iosVersion: 1705, standalone: true } })).toBe('on')
  })
})

describe('safeAppPath', () => {
  it('keeps app paths and rejects other origins', () => {
    expect(safeAppPath('/incidents?tab=sos')).toBe('/incidents?tab=sos')
    expect(safeAppPath('//evil.example/x')).toBe('/')
    expect(safeAppPath('/\\evil.example')).toBe('/')
    expect(safeAppPath('https://evil.example/')).toBe('/')
    expect(safeAppPath('javascript:alert(1)')).toBe('/')
    expect(safeAppPath(undefined)).toBe('/')
  })
})

// ── browser fakes ────────────────────────────────────────────────────────────

interface FakeSub {
  endpoint: string
  options: { applicationServerKey: ArrayBuffer | null }
  unsubscribe: ReturnType<typeof vi.fn>
  toJSON: () => { endpoint: string; keys: { p256dh: string; auth: string } }
}

let seq = 0
const manager = {
  current: null as FakeSub | null,
  getSubscription: vi.fn(async () => manager.current),
  subscribe: vi.fn(async ({ applicationServerKey }: { userVisibleOnly: boolean; applicationServerKey: Uint8Array }) => {
    manager.current = fakeSub(`https://fcm.googleapis.com/fcm/send/${++seq}`, applicationServerKey)
    return manager.current
  }),
}

function fakeSub(endpoint: string, key: Uint8Array): FakeSub {
  const sub: FakeSub = {
    endpoint,
    options: { applicationServerKey: key.slice().buffer },
    unsubscribe: vi.fn(async () => {
      if (manager.current === sub) manager.current = null
      return true
    }),
    toJSON: () => ({ endpoint, keys: { p256dh: 'p256dh-' + endpoint.slice(-1), auth: 'auth' } }),
  }
  return sub
}

const registration = { active: {}, pushManager: manager, showNotification: vi.fn() }
const notification = { permission: 'default' as NotificationPermission, requestPermission: vi.fn(async () => 'granted' as NotificationPermission) }

function fakeApi(rows: PushSubscriptionRow[] = []): PushApi & { create: ReturnType<typeof vi.fn>; remove: ReturnType<typeof vi.fn>; list: ReturnType<typeof vi.fn> } {
  return {
    create: vi.fn(async () => ({ id: 'new-id' })),
    remove: vi.fn(async () => undefined),
    list: vi.fn(async () => rows),
  }
}

const row = (id: string, endpoint: string): PushSubscriptionRow => ({
  id,
  kind: 'webpush',
  label: 'Chrome on Windows',
  endpoint,
  created_at: '2026-10-01T00:00:00Z',
  last_success_at: null,
  last_failure_at: null,
  last_error: null,
})

function httpError(status: number): AxiosError {
  const config = { headers: new AxiosHeaders() } as InternalAxiosRequestConfig
  return new AxiosError('x', 'ERR', config, {}, { status, statusText: '', data: {}, headers: {}, config } as AxiosResponse)
}

describe('enable / disable flows', () => {
  beforeEach(() => {
    seq = 0
    manager.current = null
    manager.getSubscription.mockClear()
    manager.subscribe.mockClear()
    notification.requestPermission.mockReset().mockResolvedValue('granted')
    resetPushForTests()
    Object.defineProperty(window.navigator, 'serviceWorker', {
      configurable: true,
      value: {
        getRegistration: vi.fn(async () => registration),
        register: vi.fn(async () => registration),
        ready: Promise.resolve(registration),
      },
    })
    vi.stubGlobal('PushManager', class {})
    vi.stubGlobal('Notification', notification)
  })

  afterEach(() => {
    delete (window.navigator as unknown as Record<string, unknown>).serviceWorker
    vi.unstubAllGlobals()
  })

  it('asks for permission, subscribes with the server key and registers the endpoint', async () => {
    const api = fakeApi()
    const res = await enablePush({ vapidKey: KEY, label: 'Chrome on Windows', api, demo: false })
    expect(notification.requestPermission).toHaveBeenCalledTimes(1)
    expect(manager.subscribe).toHaveBeenCalledTimes(1)
    const opts = manager.subscribe.mock.calls[0][0]
    expect(opts.userVisibleOnly).toBe(true)
    expect(Array.from(opts.applicationServerKey)).toEqual(Array.from(urlBase64ToUint8Array(KEY)))
    expect(api.create).toHaveBeenCalledWith({
      kind: 'webpush',
      endpoint: 'https://fcm.googleapis.com/fcm/send/1',
      keys: { p256dh: 'p256dh-1', auth: 'auth' },
      label: 'Chrome on Windows',
    })
    expect(res).toEqual({ id: 'new-id', endpoint: 'https://fcm.googleapis.com/fcm/send/1', simulated: false })
    expect((await browserPushInfo()).endpoint).toBe('https://fcm.googleapis.com/fcm/send/1')
  })

  it('stops when the user blocks or dismisses the prompt', async () => {
    const api = fakeApi()
    notification.requestPermission.mockResolvedValueOnce('denied')
    await expect(enablePush({ vapidKey: KEY, label: 'x', api, demo: false })).rejects.toMatchObject({ code: 'denied' })
    notification.requestPermission.mockResolvedValueOnce('default')
    await expect(enablePush({ vapidKey: KEY, label: 'x', api, demo: false })).rejects.toMatchObject({ code: 'dismissed' })
    expect(manager.subscribe).not.toHaveBeenCalled()
    expect(api.create).not.toHaveBeenCalled()
  })

  it('replaces a subscription made with an old (rotated) key and drops its server row', async () => {
    const old = fakeSub('https://fcm.googleapis.com/fcm/send/old', urlBase64ToUint8Array(OTHER_KEY))
    manager.current = old
    const api = fakeApi()
    await enablePush({ vapidKey: KEY, label: 'x', api, rows: [row('old-row', old.endpoint)], demo: false })
    expect(old.unsubscribe).toHaveBeenCalled()
    expect(api.remove).toHaveBeenCalledWith('old-row')
    expect(manager.current?.endpoint).toBe('https://fcm.googleapis.com/fcm/send/1')
    expect(keyMatches(manager.current?.options.applicationServerKey, KEY)).toBe(true)
  })

  it('does not leave an orphan subscription when the server refuses it', async () => {
    const api = fakeApi()
    api.create.mockRejectedValueOnce(httpError(409))
    await expect(enablePush({ vapidKey: KEY, label: 'x', api, demo: false })).rejects.toBeInstanceOf(AxiosError)
    expect(manager.current).toBeNull()
  })

  it('maps a failing push service to subscribe_failed, or simulates it in demo mode', async () => {
    manager.subscribe.mockRejectedValueOnce(new DOMException('push service error', 'AbortError'))
    await expect(enablePush({ vapidKey: KEY, label: 'x', api: fakeApi(), demo: false })).rejects.toMatchObject({ code: 'subscribe_failed' })

    manager.subscribe.mockRejectedValueOnce(new DOMException('push service error', 'AbortError'))
    const api = fakeApi()
    const res = await enablePush({ vapidKey: KEY, label: 'Chrome on Windows', api, demo: true })
    expect(res.simulated).toBe(true)
    expect(res.endpoint).toMatch(/^https:\/\/demo\.push\.invalid\//)
    expect(api.create.mock.calls[0][0]).toMatchObject({ kind: 'webpush', endpoint: res.endpoint })
    expect(await browserPushInfo()).toMatchObject({ endpoint: res.endpoint, simulated: true })
  })

  it('throws unsupported without the Push API', async () => {
    vi.stubGlobal('PushManager', undefined)
    delete (window as unknown as Record<string, unknown>).PushManager
    await expect(enablePush({ vapidKey: KEY, label: 'x', api: fakeApi(), demo: false })).rejects.toMatchObject({ code: 'unsupported' })
  })

  it('disable removes the server row of this browser, then unsubscribes', async () => {
    const sub = fakeSub('https://fcm.googleapis.com/fcm/send/me', urlBase64ToUint8Array(KEY))
    manager.current = sub
    const api = fakeApi()
    await disablePush({ rows: [row('other', 'https://x/other'), row('mine', sub.endpoint)], api })
    expect(api.remove).toHaveBeenCalledWith('mine')
    expect(api.remove).toHaveBeenCalledTimes(1)
    expect(sub.unsubscribe).toHaveBeenCalled()
  })

  it('disable tolerates a row that is already gone, but keeps the subscription on other errors', async () => {
    const sub = fakeSub('https://fcm.googleapis.com/fcm/send/me', urlBase64ToUint8Array(KEY))
    manager.current = sub
    const api = fakeApi()
    api.remove.mockRejectedValueOnce(httpError(500))
    await expect(disablePush({ rows: [row('mine', sub.endpoint)], api })).rejects.toBeInstanceOf(AxiosError)
    expect(sub.unsubscribe).not.toHaveBeenCalled()

    api.remove.mockRejectedValueOnce(httpError(404))
    await disablePush({ rows: [row('mine', sub.endpoint)], api })
    expect(sub.unsubscribe).toHaveBeenCalled()
  })

  it('account switch removes the server row and unsubscribes (best effort)', async () => {
    const sub = fakeSub('https://fcm.googleapis.com/fcm/send/me', urlBase64ToUint8Array(KEY))
    manager.current = sub
    const api = fakeApi([row('mine', sub.endpoint)])
    await dropPushForAccountSwitch({ api })
    expect(api.list).toHaveBeenCalled()
    expect(api.remove).toHaveBeenCalledWith('mine')
    expect(sub.unsubscribe).toHaveBeenCalled()
  })

  it('account switch without a token only unsubscribes locally, and never hangs', async () => {
    const sub = fakeSub('https://fcm.googleapis.com/fcm/send/me', urlBase64ToUint8Array(KEY))
    manager.current = sub
    const api = fakeApi()
    await dropPushForAccountSwitch({ api, server: false })
    expect(api.list).not.toHaveBeenCalled()
    expect(sub.unsubscribe).toHaveBeenCalled()

    manager.current = fakeSub('https://fcm.googleapis.com/fcm/send/again', urlBase64ToUint8Array(KEY))
    const hanging = fakeApi()
    hanging.list.mockImplementation(() => new Promise(() => undefined))
    const t0 = Date.now()
    await dropPushForAccountSwitch({ api: hanging, timeoutMs: 50 })
    expect(Date.now() - t0).toBeLessThan(1000)
  })

  it('account switch without any subscription does nothing', async () => {
    const api = fakeApi()
    await dropPushForAccountSwitch({ api })
    expect(api.list).not.toHaveBeenCalled()
  })
})
