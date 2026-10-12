import { useEffect, useState } from 'react'
import { addDays, localeOf, todayIn, type Lang } from '../../lib/time'
import type { Key, Vars } from '../../i18n/context'
import type { CheckpointStatus } from '../../types'

/** Current time, re-rendered every `intervalMs` (for "x seconds ago" labels). */
export function useNow(intervalMs = 1000): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), intervalMs)
    return () => window.clearInterval(id)
  }, [intervalMs])
  return now
}

type T = (key: Key, vars?: Vars) => string

/** Human duration: "8 mnt", "1 j 25 mnt", "2 hr 3 j". */
export function fmtDuration(ms: number, t: T): string {
  const totalMin = Math.max(0, Math.round(ms / 60_000))
  if (totalMin < 60) return t('ops.dur.m', { m: totalMin })
  const h = Math.floor(totalMin / 60)
  const m = totalMin % 60
  if (h < 24) return m ? t('ops.dur.hm', { h, m }) : t('ops.dur.h', { h })
  return t('ops.dur.dh', { d: Math.floor(h / 24), h: h % 24 })
}

/** "3 menit yang lalu" / "3 minutes ago" relative to `now`. */
export function relTime(iso: string | number, lang: Lang, now: number): string {
  const then = typeof iso === 'number' ? iso : Date.parse(iso)
  const sec = Math.round((then - now) / 1000)
  const rtf = new Intl.RelativeTimeFormat(localeOf(lang), { numeric: 'auto' })
  const abs = Math.abs(sec)
  if (abs < 60) return rtf.format(Math.abs(sec) < 5 ? 0 : sec, 'second')
  if (abs < 3600) return rtf.format(Math.round(sec / 60), 'minute')
  if (abs < 86_400) return rtf.format(Math.round(sec / 3600), 'hour')
  return rtf.format(Math.round(sec / 86_400), 'day')
}

export function initials(name: string | undefined): string {
  return (name || '?')
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((x) => x[0]?.toUpperCase())
    .join('')
}

/** Segment / dot colour per checkpoint status (green on time, amber late, orange overdue, red missed, grey pending). */
export const cpTone: Record<CheckpointStatus, { bar: string; ring: string; text: string }> = {
  on_time: { bar: 'bg-emerald-500', ring: 'bg-emerald-50 text-emerald-600 ring-emerald-200', text: 'text-emerald-700' },
  late: { bar: 'bg-amber-400', ring: 'bg-amber-50 text-amber-600 ring-amber-200', text: 'text-amber-700' },
  overdue: { bar: 'bg-orange-500', ring: 'bg-orange-50 text-orange-600 ring-orange-200', text: 'text-orange-700' },
  missed: { bar: 'bg-red-500', ring: 'bg-red-50 text-red-600 ring-red-200', text: 'text-red-700' },
  pending: { bar: 'bg-gray-200', ring: 'bg-white text-gray-500 ring-gray-200', text: 'text-gray-500' },
}

export type RangePreset = 'today' | '7d' | '30d'

export function presetRange(p: RangePreset, tz: string): { from: string; to: string } {
  const to = todayIn(tz)
  return { from: p === 'today' ? to : addDays(to, p === '7d' ? -6 : -29), to }
}

export function matchPreset(from: string, to: string, tz: string): RangePreset | null {
  for (const p of ['today', '7d', '30d'] as const) {
    const r = presetRange(p, tz)
    if (r.from === from && r.to === to) return p
  }
  return null
}
