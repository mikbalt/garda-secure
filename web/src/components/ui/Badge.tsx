import type { ReactNode } from 'react'
import { useI18n, translateMaybe } from '../../i18n/context'
import { severityTone, statusTone, type Tone } from './styles'

export type { Tone }


const tones: Record<Tone, string> = {
  gray: 'bg-gray-100 text-gray-700 ring-gray-200',
  green: 'bg-emerald-50 text-emerald-700 ring-emerald-200',
  yellow: 'bg-amber-50 text-amber-800 ring-amber-200',
  red: 'bg-red-50 text-red-700 ring-red-200',
  blue: 'bg-primary-50 text-primary-700 ring-primary-200',
  orange: 'bg-orange-50 text-orange-700 ring-orange-200',
  purple: 'bg-violet-50 text-violet-700 ring-violet-200',
}

const dots: Record<Tone, string> = {
  gray: 'bg-gray-400',
  green: 'bg-emerald-500',
  yellow: 'bg-amber-500',
  red: 'bg-red-500',
  blue: 'bg-primary-500',
  orange: 'bg-orange-500',
  purple: 'bg-violet-500',
}

/** Small coloured status dot (optionally pulsing for "live" states). */
export function Dot({ tone = 'gray', pulse = false }: { tone?: Tone; pulse?: boolean }) {
  return (
    <span className="relative inline-flex h-2 w-2 shrink-0">
      {pulse && <span className={`absolute inline-flex h-full w-full animate-ping2 rounded-full ${dots[tone]} opacity-60`} />}
      <span className={`relative inline-flex h-2 w-2 rounded-full ${dots[tone]}`} />
    </span>
  )
}

export function Badge({ tone = 'gray', children, dot = false }: { tone?: Tone; children: ReactNode; dot?: boolean }) {
  return (
    <span
      className={`inline-flex items-center gap-1.5 whitespace-nowrap rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset ${tones[tone]}`}
    >
      {dot && <Dot tone={tone} />}
      {children}
    </span>
  )
}

/** Coloured, translated badge for any status value. */
export function StatusBadge({ status }: { status: string }) {
  const { lang } = useI18n()
  return (
    <Badge tone={statusTone[status] ?? 'gray'} dot>
      {translateMaybe(lang, `status.${status}`, status)}
    </Badge>
  )
}

export function SeverityBadge({ severity }: { severity: string }) {
  const { lang } = useI18n()
  return <Badge tone={severityTone[severity] ?? 'gray'}>{translateMaybe(lang, `severity.${severity}`, severity)}</Badge>
}
