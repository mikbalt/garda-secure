import { useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowRight, BellRing } from 'lucide-react'
import { useAuth } from '../hooks/useAuth'
import { usePush } from '../hooks/usePush'
import { useI18n } from '../i18n/context'
import { readStored, STORAGE_KEYS, writeStored } from '../lib/storage'
import { buttonClass } from './ui/styles'

function Hint({ onDismiss }: { onDismiss: () => void }) {
  const { t } = useI18n()
  const push = usePush()
  // Only when it can actually be turned on here: supported, set up on the server, not blocked, not on yet.
  if (push.state !== 'off') return null
  return (
    <aside
      aria-labelledby="push-nudge-title"
      className="mb-6 flex flex-col gap-4 rounded-2xl border border-gray-200/80 bg-white p-4 shadow-card sm:flex-row sm:items-center sm:pr-5"
    >
      <div className="flex min-w-0 flex-1 items-start gap-3 sm:items-center sm:gap-4">
        <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-red-50 text-red-600 ring-1 ring-red-100">
          <BellRing className="h-5 w-5" aria-hidden="true" />
        </span>
        <div className="min-w-0 flex-1">
          <h2 id="push-nudge-title" className="text-sm font-semibold text-gray-900">
            {t('push.nudge.title')}
          </h2>
          <p className="mt-0.5 text-sm text-gray-600">{t('push.nudge.body')}</p>
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-2 sm:shrink-0">
        <Link to="/notifications" className={buttonClass('primary', 'md', 'flex-1 sm:flex-none')}>
          {t('push.nudge.cta')}
          <ArrowRight className="h-4 w-4" aria-hidden="true" />
        </Link>
        <button type="button" onClick={onDismiss} className={buttonClass('ghost', 'md')}>
          {t('push.nudge.dismiss')}
        </button>
      </div>
    </aside>
  )
}

/**
 * One-time dashboard hint for admins and supervisors to turn on push for SOS
 * alerts on this browser; "Not now" hides it for good in this browser.
 */
export function PushNudge() {
  const { can } = useAuth()
  const [dismissed, setDismissed] = useState(() => readStored(STORAGE_KEYS.pushNudgeDismissed) === '1')
  if (!can('viewSos') || dismissed) return null
  return (
    <Hint
      onDismiss={() => {
        writeStored(STORAGE_KEYS.pushNudgeDismissed, '1')
        setDismissed(true)
      }}
    />
  )
}
