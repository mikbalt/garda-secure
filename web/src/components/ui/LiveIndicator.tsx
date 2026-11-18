import { useEffect, useState } from 'react'
import { Dot } from './Badge'
import { localeOf, type Lang } from '../../lib/time'
import { useI18n } from '../../i18n/context'

/** Current timestamp, re-rendered every `intervalMs` milliseconds. */
function useNow(intervalMs = 1000): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), intervalMs)
    return () => window.clearInterval(id)
  }, [intervalMs])
  return now
}

/** "3 menit yang lalu" / "3 minutes ago" relative to `now`. */
function relTime(iso: string | number, lang: Lang, now: number): string {
  const then = typeof iso === 'number' ? iso : Date.parse(iso)
  const sec = Math.round((then - now) / 1000)
  const rtf = new Intl.RelativeTimeFormat(localeOf(lang), { numeric: 'auto' })
  const abs = Math.abs(sec)
  if (abs < 60) return rtf.format(Math.abs(sec) < 5 ? 0 : sec, 'second')
  if (abs < 3600) return rtf.format(Math.round(sec / 60), 'minute')
  if (abs < 86_400) return rtf.format(Math.round(sec / 3600), 'hour')
  return rtf.format(Math.round(sec / 86_400), 'day')
}

/** Pulsing dot + "Updated 4 seconds ago" (ticks every second). */
export function LiveIndicator({ updatedAt, fetching }: { updatedAt: number; fetching?: boolean }) {
  const { t, lang } = useI18n()
  const now = useNow(1000)
  return (
    <span className="inline-flex items-center gap-2 rounded-full bg-white px-3 py-1.5 text-xs font-medium text-gray-600 shadow-card ring-1 ring-gray-200/80">
      <Dot tone="green" pulse />
      <span aria-live="off">
        {updatedAt ? t('ops.live.updated', { when: relTime(updatedAt, lang, Math.max(now, updatedAt)) }) : t('ops.live.connecting')}
      </span>
      {fetching && <span className="sr-only">{t('app.loading')}</span>}
    </span>
  )
}
