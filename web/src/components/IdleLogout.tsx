import { useCallback, useEffect, useRef, useState } from 'react'
import { LogOut, TimerReset } from 'lucide-react'
import { config } from '../config'
import { logout } from '../keycloak'
import { useI18n } from '../i18n/context'
import { idlePhase, samePhase, type IdlePhase } from '../lib/idle'
import { readStored, STORAGE_KEYS, writeStored } from '../lib/storage'
import { Button } from './ui/Button'
import { Modal } from './ui/Modal'

/** User input that counts as activity. Background polling never does. */
const ACTIVITY_EVENTS = ['pointerdown', 'pointermove', 'keydown', 'wheel', 'touchstart'] as const
/** Activity is shared with other tabs via localStorage, at most this often. */
const SHARE_EVERY_MS = 15_000

/**
 * App idle logout (WEB-4): after `IDLE_LOGOUT_MINUTES` (runtime config,
 * default 30) without keyboard/pointer input in any tab, a warning with a
 * countdown appears; when it runs out the user is signed out of Keycloak.
 * The 10 s SOS polling keeps tokens fresh but is not activity.
 */
export function IdleLogout({ minutes = config.idleLogoutMinutes, onExpire = logout }: { minutes?: number; onExpire?: () => void }) {
  const { t } = useI18n()
  const timeoutMs = minutes * 60_000
  const last = useRef(Date.now())
  const shared = useRef(0)
  const [phase, setPhase] = useState<IdlePhase>({ kind: 'active' })
  const warning = phase.kind === 'warning'
  const warningRef = useRef(false)
  warningRef.current = warning

  const touch = useCallback((share = true) => {
    const now = Date.now()
    last.current = now
    if (share && now - shared.current > SHARE_EVERY_MS) {
      shared.current = now
      writeStored(STORAGE_KEYS.lastActivity, String(now))
    }
  }, [])

  useEffect(() => {
    if (timeoutMs <= 0) return
    // While the warning is shown, only "Stay signed in" (or another tab) resets it.
    const onActivity = () => {
      if (!warningRef.current) touch()
    }
    const onStorage = (e: StorageEvent) => {
      if (e.key !== STORAGE_KEYS.lastActivity || !e.newValue) return
      const at = Number(e.newValue)
      if (Number.isFinite(at) && at > last.current) last.current = at
    }
    for (const ev of ACTIVITY_EVENTS) window.addEventListener(ev, onActivity, { passive: true })
    window.addEventListener('storage', onStorage)
    const tick = window.setInterval(() => {
      const other = Number(readStored(STORAGE_KEYS.lastActivity))
      if (Number.isFinite(other) && other > last.current) last.current = other
      const p = idlePhase(Date.now(), last.current, timeoutMs)
      setPhase((prev) => (samePhase(prev, p) ? prev : p))
    }, 1000)
    return () => {
      for (const ev of ACTIVITY_EVENTS) window.removeEventListener(ev, onActivity)
      window.removeEventListener('storage', onStorage)
      window.clearInterval(tick)
    }
  }, [timeoutMs, touch])

  const expired = phase.kind === 'expired'
  useEffect(() => {
    if (expired) onExpire()
  }, [expired, onExpire])

  if (timeoutMs <= 0 || phase.kind !== 'warning') return null

  const stay = () => {
    shared.current = 0
    touch()
    setPhase({ kind: 'active' })
  }

  return (
    <Modal
      open
      onClose={stay}
      title={t('idle.title')}
      footer={
        <>
          <Button variant="secondary" onClick={() => onExpire()}>
            <LogOut className="h-4 w-4" aria-hidden="true" />
            {t('user.logout')}
          </Button>
          <Button onClick={stay} data-autofocus>
            <TimerReset className="h-4 w-4" aria-hidden="true" />
            {t('idle.stay')}
          </Button>
        </>
      }
    >
      <p className="text-sm text-gray-700">{t('idle.body', { minutes })}</p>
      {/* role=timer is not announced every second (implicit aria-live off). */}
      <p role="timer" className="mt-3 text-2xl font-semibold tabular-nums text-gray-900">
        {t('idle.countdown', { s: phase.secondsLeft })}
      </p>
    </Modal>
  )
}
