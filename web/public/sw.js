/*
 * GARDA service worker: web push notifications only.
 *
 * Deliberately no fetch handler and no caching — the app is always loaded
 * from the network, so a new release can never be hidden behind a stale
 * cached copy. Served from the site root (scope "/"), same origin, no
 * inline script (CSP worker-src 'self').
 *
 * Payload (JSON, docs/api.md): {title, body, url, tag, event, severity, site_id};
 * `url` is an app-relative deep link such as "/incidents?tab=sos".
 */

self.addEventListener('install', () => {
  self.skipWaiting()
})

self.addEventListener('activate', (event) => {
  event.waitUntil(self.clients.claim())
})

/** Only same-origin app paths ("/x", not "//host" or "/\\host"); anything else opens "/". */
function safePath(url) {
  return typeof url === 'string' && /^\/(?![/\\])/.test(url) ? url : '/'
}

function parse(data) {
  if (!data) return {}
  try {
    const json = data.json()
    return json && typeof json === 'object' ? json : {}
  } catch {
    try {
      return { body: data.text() }
    } catch {
      return {}
    }
  }
}

self.addEventListener('push', (event) => {
  const p = parse(event.data)
  const title = typeof p.title === 'string' && p.title ? p.title : 'GARDA'
  const options = {
    body: typeof p.body === 'string' ? p.body : '',
    icon: '/icons/icon-192.png',
    badge: '/icons/favicon-32.png',
    data: { url: safePath(p.url), event: p.event || null, site_id: p.site_id || null },
    // Critical alerts (SOS) stay on screen until the user reacts.
    requireInteraction: p.severity === 'critical',
  }
  const tag = typeof p.tag === 'string' && p.tag ? p.tag : null
  if (tag) {
    options.tag = tag
    // A repeated SOS reminder with the same tag must alert again, not replace silently.
    options.renotify = p.event === 'sos' || p.event === 'sos_unacknowledged'
  }
  event.waitUntil(self.registration.showNotification(title, options))
})

self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  const path = safePath(event.notification.data && event.notification.data.url)
  const target = new URL(path, self.location.origin).href
  event.waitUntil(
    (async () => {
      const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true })
      for (const client of windows) {
        if (new URL(client.url).origin !== self.location.origin) continue
        try {
          const focused = await client.focus()
          // The app navigates inside the SPA (keeps the session, no reload).
          focused.postMessage({ type: 'garda:navigate', url: path })
          return
        } catch {
          // Not allowed to focus this one; try the next window or open a new one.
        }
      }
      await self.clients.openWindow(target)
    })()
  )
})
