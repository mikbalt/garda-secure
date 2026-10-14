import React from 'react'
import ReactDOM from 'react-dom/client'
import { I18nProvider } from './i18n/I18nProvider'
import { Bootstrap } from './bootstrap/Bootstrap'
import { registerServiceWorker } from './lib/push'
import './index.css'

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <I18nProvider>
      <Bootstrap />
    </I18nProvider>
  </React.StrictMode>
)

// Push notifications (public/sw.js: push + notification clicks only, no caching).
// Registering also picks up a new sw.js after a release.
if (import.meta.env.MODE !== 'test') {
  if (document.readyState === 'complete') void registerServiceWorker()
  else window.addEventListener('load', () => void registerServiceWorker(), { once: true })
}
