import type { ReactNode } from 'react'
import { useI18n, translateMaybe } from '../../i18n/context'

type Tone = 'gray' | 'green' | 'yellow' | 'red' | 'blue' | 'orange' | 'purple'

const tones: Record<Tone, string> = {
  gray: 'bg-gray-100 text-gray-800 ring-gray-300',
  green: 'bg-green-100 text-green-900 ring-green-300',
  yellow: 'bg-yellow-100 text-yellow-900 ring-yellow-300',
  red: 'bg-red-100 text-red-900 ring-red-300',
  blue: 'bg-primary-100 text-primary-900 ring-primary-300',
  orange: 'bg-orange-100 text-orange-900 ring-orange-300',
  purple: 'bg-purple-100 text-purple-900 ring-purple-300',
}

export function Badge({ tone = 'gray', children }: { tone?: Tone; children: ReactNode }) {
  return (
    <span className={`inline-flex items-center gap-1 whitespace-nowrap rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset ${tones[tone]}`}>
      {children}
    </span>
  )
}

const statusTone: Record<string, Tone> = {
  present: 'green',
  on_time: 'green',
  sent: 'green',
  succeeded: 'green',
  completed: 'green',
  resolved: 'green',
  active: 'green',
  late: 'yellow',
  in_progress: 'blue',
  running: 'blue',
  queued: 'blue',
  pending: 'gray',
  inactive: 'gray',
  open: 'orange',
  overdue: 'orange',
  absent: 'red',
  missed: 'red',
  failed: 'red',
  revoked: 'red',
  abandoned: 'purple',
}

/** Coloured, translated badge for any status value. */
export function StatusBadge({ status }: { status: string }) {
  const { lang } = useI18n()
  return <Badge tone={statusTone[status] ?? 'gray'}>{translateMaybe(lang, `status.${status}`, status)}</Badge>
}

const severityTone: Record<string, Tone> = { low: 'gray', medium: 'yellow', high: 'orange', critical: 'red' }

export function SeverityBadge({ severity }: { severity: string }) {
  const { lang } = useI18n()
  return <Badge tone={severityTone[severity] ?? 'gray'}>{translateMaybe(lang, `severity.${severity}`, severity)}</Badge>
}
