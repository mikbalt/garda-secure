import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer, Cell } from 'recharts'
import {
  CalendarCheck,
  UserCheck,
  UserX,
  MapPin,
  AlertTriangle,
  Siren,
  Wrench,
  ClipboardCheck,
  type LucideIcon,
} from 'lucide-react'
import { CardSkeleton } from '../components/LoadingSkeleton'
import { RequireSite } from '../components/RequireSite'
import { Card, ErrorState, PageHeader } from '../components/ui/States'
import { apiGet } from '../api/client'
import { useI18n, type Key } from '../i18n/context'
import { useAuth } from '../hooks/useAuth'
import { formatDay } from '../lib/time'
import type { Capability } from '../lib/roles'
import type { DashboardSummary, Severity, Site } from '../types'

interface StatDef {
  key: keyof DashboardSummary
  label: Key
  icon: LucideIcon
  tone: string
  to?: string
  cap?: Capability
  alert?: boolean
}

const stats: StatDef[] = [
  { key: 'shifts_today', label: 'dash.shiftsToday', icon: CalendarCheck, tone: 'bg-primary-50 text-primary-700', to: '/attendance' },
  { key: 'attended_today', label: 'dash.attendedToday', icon: UserCheck, tone: 'bg-green-50 text-green-700', to: '/attendance' },
  { key: 'absent_today', label: 'dash.absentToday', icon: UserX, tone: 'bg-red-50 text-red-700', to: '/attendance', alert: true },
  { key: 'active_patrols', label: 'dash.activePatrols', icon: MapPin, tone: 'bg-green-50 text-green-700', to: '/patrol', cap: 'viewPatrol' },
  { key: 'open_incidents', label: 'dash.openIncidents', icon: AlertTriangle, tone: 'bg-orange-50 text-orange-700', to: '/incidents', cap: 'viewIncidents' },
  { key: 'sos_alerts_unacknowledged', label: 'dash.unackSos', icon: Siren, tone: 'bg-red-50 text-red-700', to: '/incidents?tab=sos', cap: 'viewSos', alert: true },
  { key: 'open_defects', label: 'dash.openDefects', icon: Wrench, tone: 'bg-yellow-50 text-yellow-800', to: '/incidents?tab=defects', cap: 'viewDefects' },
  { key: 'attendance_pending_review', label: 'dash.pendingReview', icon: ClipboardCheck, tone: 'bg-purple-50 text-purple-700', to: '/attendance?tab=review', cap: 'reviewAttendance' },
]

const severityColors: Record<Severity, string> = {
  low: '#6b7280',
  medium: '#ca8a04',
  high: '#ea580c',
  critical: '#b91c1c',
}

function StatCard({ def, value }: { def: StatDef; value: number }) {
  const { t } = useI18n()
  const { can } = useAuth()
  const Icon = def.icon
  const highlight = def.alert && value > 0
  const body = (
    <div
      className={`flex h-full items-start gap-4 rounded-xl border bg-white p-5 shadow-sm ${
        highlight ? 'border-red-300 ring-1 ring-red-200' : 'border-gray-200'
      }`}
    >
      <div className={`rounded-lg p-3 ${def.tone}`}>
        <Icon className="h-5 w-5" aria-hidden="true" />
      </div>
      <div>
        <p className="text-sm text-gray-700">{t(def.label)}</p>
        <p className="mt-0.5 text-2xl font-bold tabular-nums text-gray-900">{value}</p>
      </div>
    </div>
  )
  const linkable = def.to && (!def.cap || can(def.cap))
  return linkable ? (
    <Link to={def.to!} className="block rounded-xl focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-600">
      {body}
    </Link>
  ) : (
    body
  )
}

function DashboardContent({ site }: { site: Site }) {
  const { t, lang } = useI18n()
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['dashboard', site.id],
    queryFn: () => apiGet<DashboardSummary>('/dashboard/summary', { site_id: site.id }),
    refetchInterval: 30_000,
  })

  if (error) return <ErrorState error={error} onRetry={() => refetch()} />

  const weekly = (data?.weekly_attendance ?? []).map((d) => ({
    ...d,
    label: formatDay(d.date, lang).replace(/ \d{4}$/, ''),
  }))
  const severities: Severity[] = ['low', 'medium', 'high', 'critical']
  const bySeverity = severities.map((s) => ({
    severity: s,
    label: t(`severity.${s}` as Key),
    count: data?.incidents_by_severity?.[s] ?? 0,
  }))

  return (
    <>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {isLoading || !data
          ? Array.from({ length: 8 }).map((_, i) => <CardSkeleton key={i} />)
          : stats.map((s) => <StatCard key={s.key} def={s} value={Number(data[s.key] ?? 0)} />)}
      </div>

      <div className="mt-6 grid grid-cols-1 gap-6 xl:grid-cols-3">
        <Card title={t('dash.weeklyAttendance')} className="xl:col-span-2">
          <div className="h-72" role="img" aria-label={t('dash.weeklyAttendance')}>
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={weekly} margin={{ top: 4, right: 8, left: -16, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
                <XAxis dataKey="label" tick={{ fontSize: 12, fill: '#374151' }} />
                <YAxis allowDecimals={false} tick={{ fontSize: 12, fill: '#374151' }} />
                <Tooltip />
                <Legend />
                <Bar dataKey="present" name={t('status.present')} stackId="a" fill="#15803d" />
                <Bar dataKey="late" name={t('status.late')} stackId="a" fill="#ca8a04" />
                <Bar dataKey="absent" name={t('status.absent')} stackId="a" fill="#b91c1c" radius={[4, 4, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </div>
        </Card>
        <Card title={t('dash.incidentsBySeverity')}>
          <div className="h-72" role="img" aria-label={t('dash.incidentsBySeverity')}>
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={bySeverity} layout="vertical" margin={{ top: 4, right: 16, left: 8, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
                <XAxis type="number" allowDecimals={false} tick={{ fontSize: 12, fill: '#374151' }} />
                <YAxis type="category" dataKey="label" width={70} tick={{ fontSize: 12, fill: '#374151' }} />
                <Tooltip />
                <Bar dataKey="count" name={t('common.total')} radius={[0, 4, 4, 0]}>
                  {bySeverity.map((s) => (
                    <Cell key={s.severity} fill={severityColors[s.severity]} />
                  ))}
                </Bar>
              </BarChart>
            </ResponsiveContainer>
          </div>
        </Card>
      </div>
    </>
  )
}

export default function Dashboard() {
  const { t } = useI18n()
  return (
    <div>
      <PageHeader title={t('dash.title')} subtitle={t('dash.autoRefresh')} />
      <RequireSite>{(site) => <DashboardContent site={site} />}</RequireSite>
    </div>
  )
}
