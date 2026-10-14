/** Push notifications (GET/POST /push/*, docs/api.md). */

export interface PushConfig {
  webpush: { enabled: boolean; vapid_public_key: string }
  fcm: { enabled: boolean }
}

/** One browser (or tablet) of the caller that receives push notifications. */
export interface PushSubscriptionRow {
  id: string
  kind: 'webpush' | string
  label: string
  endpoint: string
  created_at: string
  last_success_at: string | null
  last_failure_at: string | null
  last_error: string | null
}

export interface CreatePushSubscription {
  kind: 'webpush'
  endpoint: string
  keys: { p256dh: string; auth: string }
  label: string
}

/** `events` = effective on/off per available event; `available` = what the caller's roles can receive. */
export interface PushPreferences {
  events: Record<string, boolean>
  available: string[]
}

export interface PushTestResult {
  sent: number
  failed: number
}

/** Payload the backend sends to the service worker (public/sw.js). */
export interface PushPayload {
  title: string
  body: string
  url: string
  tag?: string
  event?: string
  severity?: 'critical' | 'high' | 'normal'
  site_id?: string
}
