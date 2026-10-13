import { CalendarCheck, CalendarClock, CalendarRange, Route as RouteIcon, ShieldAlert, Trophy, type LucideIcon } from 'lucide-react'
import type { ExportFormat } from '../../components/exportFormat'
import type { Key } from '../../i18n/context'
import type { Capability } from '../../lib/roles'

export type ReportKey = 'daily' | 'monthly' | 'patrol' | 'compliance' | 'incidents' | 'kpi'

export type ReportTone = 'green' | 'blue' | 'violet' | 'orange' | 'amber' | 'teal'

export interface ReportDef {
  key: ReportKey
  cap: Capability
  icon: LucideIcon
  tone: ReportTone
  /** Report name. */
  title: Key
  /** The question the report answers (card body + page subtitle). */
  question: Key
  /** Who typically uses it. */
  audience: Key
  /** Download formats (D-045); every report also prints. */
  formats: ExportFormat[]
}

/** The reports in the order they are shown on the landing page. */
export const REPORTS: ReportDef[] = [
  { key: 'daily', cap: 'reportAttendanceDaily', icon: CalendarCheck, tone: 'green', title: 'adm.rep.daily.title', question: 'adm.rep.daily.q', audience: 'adm.rep.daily.who', formats: ['xlsx', 'csv'] },
  { key: 'monthly', cap: 'reportAttendanceMonthly', icon: CalendarRange, tone: 'blue', title: 'adm.rep.monthly.title', question: 'adm.rep.monthly.q', audience: 'adm.rep.monthly.who', formats: ['xlsx', 'csv'] },
  { key: 'patrol', cap: 'reportPatrol', icon: RouteIcon, tone: 'violet', title: 'adm.rep.patrol.title', question: 'adm.rep.patrol.q', audience: 'adm.rep.patrol.who', formats: ['xlsx', 'csv'] },
  { key: 'compliance', cap: 'reportPatrol', icon: CalendarClock, tone: 'teal', title: 'p2.rep.comp.title', question: 'p2.rep.comp.q', audience: 'p2.rep.comp.who', formats: ['xlsx', 'csv'] },
  { key: 'incidents', cap: 'reportIncidents', icon: ShieldAlert, tone: 'orange', title: 'adm.rep.incidents.title', question: 'adm.rep.incidents.q', audience: 'adm.rep.incidents.who', formats: ['xlsx', 'csv', 'pdf'] },
  { key: 'kpi', cap: 'reportKpi', icon: Trophy, tone: 'amber', title: 'adm.rep.kpi.title', question: 'adm.rep.kpi.q', audience: 'adm.rep.kpi.who', formats: ['xlsx', 'csv'] },
]

export const toneClass: Record<ReportTone, string> = {
  green: 'bg-emerald-50 text-emerald-600 ring-emerald-100',
  blue: 'bg-primary-50 text-primary-600 ring-primary-100',
  violet: 'bg-violet-50 text-violet-600 ring-violet-100',
  orange: 'bg-orange-50 text-orange-600 ring-orange-100',
  amber: 'bg-amber-50 text-amber-600 ring-amber-100',
  teal: 'bg-teal-50 text-teal-600 ring-teal-100',
}
