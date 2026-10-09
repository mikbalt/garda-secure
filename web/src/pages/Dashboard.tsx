import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import {
  ArrowRight,
  CheckCircle2,
  ClipboardCheck,
  Clock4,
  ListChecks,
  Route as RouteIcon,
  ShieldAlert,
  Tablet,
  TimerOff,
  UserCheck,
  UserX,
  Wrench,
  type LucideIcon,
} from 'lucide-react'
import { RequireSite } from '../components/RequireSite'
import { CardSkeleton } from '../components/LoadingSkeleton'
import { Card, ErrorState } from '../components/ui/States'
import { Badge, Dot, SeverityBadge, StatusBadge } from '../components/ui/Badge'
import { buttonClass, type Tone } from '../components/ui/styles'
import { apiGet } from '../api/client'
import { useAuth } from '../hooks/useAuth'
import { useSetupProgress } from '../hooks/useSetupProgress'
import { incidentTypeLabel, useI18n, type Key } from '../i18n/context'
import { addDays, formatDay, formatTime, localTimeOf, todayIn } from '../lib/time'
import type { DailyAttendance, DashboardSummary, Device, Incident, LivePatrol, Site } from '../types'

// ── small building blocks ────────────────────────────────────────────────────

function greetingKey(tz: string): Key {
  const h = Number(localTimeOf(new Date(), tz).slice(0, 2))
  if (h < 11) return 'today.morning'
  if (h < 15) return 'today.afternoon'
  if (h < 18) return 'today.evening'
  return 'today.night'
}

interface KpiProps {
  label: string
  value: string | number
  sub: string
  icon: LucideIcon
  tone: 'blue' | 'green' | 'orange' | 'amber'
  to?: string
  progress?: number
}

const kpiTone = {
  blue: 'bg-primary-50 text-primary-600 ring-primary-100',
  green: 'bg-emerald-50 text-emerald-600 ring-emerald-100',
  orange: 'bg-orange-50 text-orange-600 ring-orange-100',
  amber: 'bg-amber-50 text-amber-600 ring-amber-100',
}

function Kpi({ label, value, sub, icon: Icon, tone, to, progress }: KpiProps) {
  const body = (
    <div className="group flex h-full flex-col rounded-2xl border border-gray-200/80 bg-white p-4 shadow-card transition-shadow hover:shadow-raised sm:p-5">
      <div className="flex items-center justify-between">
        <span className={`flex h-9 w-9 items-center justify-center rounded-xl ring-1 ${kpiTone[tone]}`}>
          <Icon className="h-[18px] w-[18px]" aria-hidden="true" />
        </span>
        {to && <ArrowRight className="h-4 w-4 text-gray-300 transition-colors group-hover:text-gray-500" aria-hidden="true" />}
      </div>
      <p className="mt-3 text-sm font-medium text-gray-500 sm:mt-4">{label}</p>
      <p className="mt-1 text-2xl font-semibold tabular-nums tracking-tight text-gray-900 sm:text-3xl">{value}</p>
      {progress !== undefined && (
        <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-gray-100">
          <div className="h-full rounded-full bg-emerald-500 transition-[width] duration-700" style={{ width: `${Math.min(100, progress)}%` }} />
        </div>
      )}
      <p className="mt-2 hidden text-xs text-gray-500 sm:block">{sub}</p>
    </div>
  )
  return to ? (
    <Link to={to} className="block rounded-2xl focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500">
      {body}
    </Link>
  ) : (
    body
  )
}

interface ActionItem {
  id: string
  tone: Tone
  icon: LucideIcon
  title: string
  hint?: string
  to: string
  cta: string
}

const actionTone: Record<string, string> = {
  red: 'bg-red-50 text-red-600 ring-red-100',
  orange: 'bg-orange-50 text-orange-600 ring-orange-100',
  yellow: 'bg-amber-50 text-amber-600 ring-amber-100',
  blue: 'bg-primary-50 text-primary-600 ring-primary-100',
  purple: 'bg-violet-50 text-violet-600 ring-violet-100',
  gray: 'bg-gray-100 text-gray-600 ring-gray-200',
  green: 'bg-emerald-50 text-emerald-600 ring-emerald-100',
}

function SetupCallout() {
  const { t } = useI18n()
  const s = useSetupProgress()
  if (!s.enabled || s.isLoading || s.complete || !s.next) return null
  const NextIcon = s.next.icon
  return (
    <div className="mb-6 flex flex-wrap items-center gap-4 rounded-2xl border border-primary-100 bg-gradient-to-r from-primary-50 via-white to-white p-4 pr-5 shadow-card">
      <span className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-primary-600 text-white shadow-lg shadow-primary-600/25">
        <ListChecks className="h-5 w-5" aria-hidden="true" />
      </span>
      <div className="min-w-0 flex-1">
        <p className="text-sm font-semibold text-gray-900">{t('setup.cardTitle', { site: s.site?.name ?? '' })}</p>
        <p className="mt-0.5 flex items-center gap-1.5 text-sm text-gray-600">
          <NextIcon className="h-4 w-4 text-primary-500" aria-hidden="true" />
          {t('setup.cardBody', { step: t(s.next.title) })}
          <span className="text-gray-400">· {t('setup.progress', { done: s.done, total: s.total })}</span>
        </p>
      </div>
      <Link to={s.next.to} className={buttonClass('primary', 'md')}>
        {t(s.next.cta)}
        <ArrowRight className="h-4 w-4" aria-hidden="true" />
      </Link>
      <Link to="/setup" className={buttonClass('ghost', 'md')}>
        {t('nav.setup')}
      </Link>
    </div>
  )
}

// ── the page ─────────────────────────────────────────────────────────────────

function Today({ site }: { site: Site }) {
  const { t, lang } = useI18n()
  const { can } = useAuth()
  const tz = site.timezone || 'Asia/Jakarta'
  const today = todayIn(tz)

  const summary = useQuery({
    queryKey: ['dashboard', site.id],
    queryFn: () => apiGet<DashboardSummary>('/dashboard/summary', { site_id: site.id }),
    refetchInterval: 30_000,
  })
  const daily = useQuery({
    queryKey: ['attendance-daily', site.id, today],
    queryFn: () => apiGet<DailyAttendance>('/attendance/daily', { site_id: site.id, date: today }),
    refetchInterval: 60_000,
  })
  const live = useQuery({
    queryKey: ['patrol-live', site.id],
    queryFn: () => apiGet<LivePatrol[]>('/patrol/live', { site_id: site.id }),
    enabled: can('viewPatrol'),
    refetchInterval: 10_000,
  })
  const incidents = useQuery({
    queryKey: ['incidents', site.id, 'recent'],
    queryFn: () => apiGet<Incident[]>('/incidents', { site_id: site.id, from: addDays(today, -6), to: today }),
    enabled: can('viewIncidents'),
    refetchInterval: 60_000,
  })
  const devices = useQuery({
    queryKey: ['devices'],
    queryFn: () => apiGet<Device[]>('/devices'),
    enabled: can('viewSettings'),
  })

  const s = summary.data
  const now = Date.now()

  const onDuty = useMemo(
    () => (daily.data?.rows ?? []).filter((r) => Date.parse(r.shift_start) <= now && now < Date.parse(r.shift_end)),
    [daily.data, now]
  )

  const actions: ActionItem[] = []
  for (const p of live.data ?? []) {
    for (const cp of p.progress.checkpoints.filter((c) => c.status === 'overdue')) {
      actions.push({
        id: `overdue-${p.progress.session.id}-${cp.id}`, tone: 'orange', icon: TimerOff,
        title: t('today.act.overdue', { cp: cp.name }), hint: t('today.act.overdueHint', { guard: p.guard_name, route: p.progress.route_name }),
        to: '/patrol', cta: t('today.act.view'),
      })
    }
  }
  if (s && s.absent_today > 0) {
    actions.push({ id: 'absent', tone: 'red', icon: UserX, title: t('today.act.absent', { n: s.absent_today }), to: '/attendance', cta: t('today.act.view') })
  }
  if (s && s.attendance_pending_review > 0 && can('reviewAttendance')) {
    actions.push({ id: 'review', tone: 'purple', icon: ClipboardCheck, title: t('today.act.review', { n: s.attendance_pending_review }), to: '/attendance?tab=review', cta: t('today.act.review.cta') })
  }
  for (const i of (incidents.data ?? []).filter((x) => x.status !== 'resolved' && (x.severity === 'high' || x.severity === 'critical'))) {
    actions.push({
      id: `inc-${i.id}`, tone: i.severity === 'critical' ? 'red' : 'orange', icon: ShieldAlert,
      title: t('today.act.incident', { severity: t(`severity.${i.severity}` as Key), type: incidentTypeLabel(lang, i.type) }),
      hint: [i.location, i.guard_name].filter(Boolean).join(' · '), to: '/incidents', cta: t('today.act.view'),
    })
  }
  const pendingDevices = (devices.data ?? []).filter((d) => d.status === 'pending').length
  if (pendingDevices > 0) {
    actions.push({ id: 'devices', tone: 'blue', icon: Tablet, title: t('today.act.device', { n: pendingDevices }), to: '/settings?tab=devices', cta: t('today.act.approve') })
  }

  const highOpen = (s?.incidents_by_severity?.high ?? 0) + (s?.incidents_by_severity?.critical ?? 0)
  const weekly = (s?.weekly_attendance ?? []).map((d) => ({ ...d, label: formatDay(d.date, lang, false).replace(/ \d{4}$/, '') }))
  const loadingTop = summary.isLoading || !s
  const kpiCount = 1 + Number(can('viewPatrol')) + Number(can('viewIncidents')) + Number(can('viewDefects'))
  const canAct = can('manageIncidents') || can('reviewAttendance') || can('viewSettings')

  if (summary.error) return <ErrorState error={summary.error} onRetry={() => summary.refetch()} />

  return (
    <>
      {/* Header */}
      <div className="mb-6 flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight text-gray-900 sm:text-[28px]">{t(greetingKey(tz))}</h1>
          <p className="mt-1 text-sm text-gray-500">{t('today.subtitle', { site: site.name, date: formatDay(today, lang) })}</p>
        </div>
        <span className="inline-flex items-center gap-2 rounded-full bg-white px-3 py-1.5 text-xs font-medium text-gray-600 shadow-card ring-1 ring-gray-200/80">
          <Dot tone="green" pulse />
          {t('today.live')}
        </span>
      </div>

      <SetupCallout />

      {/* KPIs */}
      <div className={`grid grid-cols-2 gap-3 sm:gap-4 ${kpiCount >= 4 ? 'xl:grid-cols-4' : 'xl:grid-cols-3'}`}>
        {loadingTop ? (
          Array.from({ length: 4 }).map((_, i) => <CardSkeleton key={i} />)
        ) : (
          <>
            <Kpi
              label={t('today.kpi.attendance')}
              value={s.shifts_today ? `${s.attended_today}/${s.shifts_today}` : '—'}
              sub={t('today.kpi.attendanceSub', { a: s.attended_today, n: s.shifts_today })}
              icon={UserCheck}
              tone="green"
              to="/attendance"
              progress={s.shifts_today ? (s.attended_today / s.shifts_today) * 100 : 0}
            />
            {can('viewPatrol') && (
              <Kpi label={t('today.kpi.patrols')} value={s.active_patrols} sub={t('today.kpi.patrolsSub')} icon={RouteIcon} tone="blue" to="/patrol" />
            )}
            {can('viewIncidents') && (
              <Kpi
                label={t('today.kpi.incidents')}
                value={s.open_incidents}
                sub={t('today.kpi.incidentsSub', { n: highOpen })}
                icon={ShieldAlert}
                tone="orange"
                to="/incidents"
              />
            )}
            {can('viewDefects') && (
              <Kpi label={t('today.kpi.defects')} value={s.open_defects} sub={t('today.kpi.defectsSub')} icon={Wrench} tone="amber" to="/incidents?tab=defects" />
            )}
          </>
        )}
      </div>

      <div className="mt-6 grid grid-cols-1 gap-6 xl:grid-cols-3">
        <div className="space-y-6 xl:col-span-2">
          {/* Needs attention */}
          <Card title={t(canAct ? 'today.needsAction' : 'today.highlights')} flush>
            {actions.length === 0 ? (
              <div className="flex items-center gap-3 px-5 py-6 text-sm text-gray-600">
                <CheckCircle2 className="h-5 w-5 text-emerald-500" aria-hidden="true" />
                {t('today.allClear')}
              </div>
            ) : (
              <ul className="divide-y divide-gray-100">
                {actions.map((a) => (
                  <li key={a.id} className="flex items-center gap-4 px-5 py-3.5">
                    <span className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-xl ring-1 ${actionTone[a.tone]}`}>
                      <a.icon className="h-[18px] w-[18px]" aria-hidden="true" />
                    </span>
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium text-gray-900">{a.title}</p>
                      {a.hint && <p className="truncate text-xs text-gray-500">{a.hint}</p>}
                    </div>
                    <Link to={a.to} className={buttonClass('secondary', 'sm')}>
                      {a.cta}
                    </Link>
                  </li>
                ))}
              </ul>
            )}
          </Card>

          {/* Live patrols */}
          {can('viewPatrol') && (
            <Card
              title={t('today.livePatrols')}
              actions={
                <Link to="/patrol" className="text-sm font-medium text-primary-600 hover:text-primary-700">
                  {t('today.viewAll')}
                </Link>
              }
            >
              {(live.data ?? []).length === 0 ? (
                <p className="text-sm text-gray-500">{t('today.livePatrolsEmpty')}</p>
              ) : (
                <div className="grid gap-3 md:grid-cols-2">
                  {(live.data ?? []).map((p) => {
                    const next = p.progress.next_index !== undefined ? p.progress.checkpoints[p.progress.next_index] : undefined
                    const overdue = p.progress.checkpoints.some((c) => c.status === 'overdue')
                    return (
                      <div key={p.progress.session.id} className="rounded-xl border border-gray-200/80 p-4">
                        <div className="flex items-start justify-between gap-2">
                          <div className="min-w-0">
                            <p className="truncate text-sm font-semibold text-gray-900">{p.guard_name}</p>
                            <p className="truncate text-xs text-gray-500">
                              {p.progress.route_name} · {formatTime(p.progress.session.started_at, tz)}
                            </p>
                          </div>
                          {overdue ? <StatusBadge status="overdue" /> : <Badge tone="blue" dot>{t('status.in_progress')}</Badge>}
                        </div>
                        <div className="mt-3 flex items-center gap-1" aria-label={t('today.checkpoints', { a: p.progress.scanned, n: p.progress.total })}>
                          {p.progress.checkpoints.map((c) => (
                            <span
                              key={c.id}
                              title={c.name}
                              className={`h-1.5 flex-1 rounded-full ${
                                c.status === 'on_time' ? 'bg-emerald-500' : c.status === 'late' ? 'bg-amber-400' : c.status === 'overdue' ? 'bg-orange-500' : 'bg-gray-200'
                              }`}
                            />
                          ))}
                        </div>
                        <div className="mt-2 flex items-center justify-between text-xs text-gray-500">
                          <span>{t('today.checkpoints', { a: p.progress.scanned, n: p.progress.total })}</span>
                          {next && <span className="truncate pl-2">{t('today.next', { cp: next.name })}</span>}
                        </div>
                      </div>
                    )
                  })}
                </div>
              )}
            </Card>
          )}

          {/* On duty */}
          <Card
            title={t('today.onDuty')}
            flush
            actions={
              <Link to="/attendance" className="text-sm font-medium text-primary-600 hover:text-primary-700">
                {t('today.viewAll')}
              </Link>
            }
          >
            {daily.isLoading ? (
              <div className="p-5">
                <div className="h-24 animate-pulse rounded-xl bg-gray-100" />
              </div>
            ) : onDuty.length === 0 ? (
              <p className="px-5 py-6 text-sm text-gray-500">{t('today.onDutyEmpty')}</p>
            ) : (
              <ul className="divide-y divide-gray-100">
                {onDuty.map((r) => (
                  <li key={r.shift_id} className="flex items-center gap-3 px-5 py-3">
                    <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-gray-100 text-xs font-semibold text-gray-600">
                      {r.guard_name
                        .split(' ')
                        .slice(0, 2)
                        .map((x) => x[0])
                        .join('')}
                    </span>
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium text-gray-900">{r.guard_name}</p>
                      <p className="flex items-center gap-1 text-xs text-gray-500">
                        <Clock4 className="h-3 w-3" aria-hidden="true" />
                        {t('today.shift', { start: formatTime(r.shift_start, tz), end: formatTime(r.shift_end, tz) })}
                        {r.clock_in_at && <span className="text-gray-400">· in {formatTime(r.clock_in_at, tz)}</span>}
                      </p>
                    </div>
                    {r.status === 'pending' ? <Badge tone="gray" dot>{t('today.notIn')}</Badge> : <StatusBadge status={r.status} />}
                  </li>
                ))}
              </ul>
            )}
          </Card>
        </div>

        <div className="space-y-6">
          {can('viewIncidents') && (
            <Card
              title={t('today.recentIncidents')}
              flush
              actions={
                <Link to="/incidents" className="text-sm font-medium text-primary-600 hover:text-primary-700">
                  {t('today.viewAll')}
                </Link>
              }
            >
              {(incidents.data ?? []).length === 0 ? (
                <p className="px-5 py-6 text-sm text-gray-500">{t('today.recentIncidentsEmpty')}</p>
              ) : (
                <ul className="divide-y divide-gray-100">
                  {(incidents.data ?? []).slice(0, 5).map((i) => (
                    <li key={i.id} className="px-5 py-3">
                      <div className="flex items-center justify-between gap-2">
                        <p className="truncate text-sm font-medium text-gray-900">{incidentTypeLabel(lang, i.type)}</p>
                        <SeverityBadge severity={i.severity} />
                      </div>
                      <p className="mt-0.5 truncate text-xs text-gray-500">
                        {[i.location, i.guard_name, formatDay(i.occurred_at.slice(0, 10), lang, false)].filter(Boolean).join(' · ')}
                      </p>
                    </li>
                  ))}
                </ul>
              )}
            </Card>
          )}

          <Card title={t('today.weekly')}>
            <div className="h-56" role="img" aria-label={t('today.weekly')}>
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={weekly} margin={{ top: 4, right: 0, left: -28, bottom: 0 }} barCategoryGap="28%">
                  <CartesianGrid vertical={false} stroke="#eef2f6" />
                  <XAxis dataKey="label" tickLine={false} axisLine={false} tick={{ fontSize: 11, fill: '#64748b' }} />
                  <YAxis allowDecimals={false} tickLine={false} axisLine={false} tick={{ fontSize: 11, fill: '#94a3b8' }} />
                  <Tooltip cursor={{ fill: '#f1f5f9' }} contentStyle={{ borderRadius: 12, border: '1px solid #e2e8f0', fontSize: 12 }} />
                  <Bar dataKey="present" name={t('status.present')} stackId="a" fill="#10b981" />
                  <Bar dataKey="late" name={t('status.late')} stackId="a" fill="#f59e0b" />
                  <Bar dataKey="absent" name={t('status.absent')} stackId="a" fill="#ef4444" radius={[4, 4, 0, 0]} />
                </BarChart>
              </ResponsiveContainer>
            </div>
            <div className="mt-3 flex items-center justify-center gap-4 text-xs text-gray-500">
              <span className="inline-flex items-center gap-1.5"><Dot tone="green" />{t('status.present')}</span>
              <span className="inline-flex items-center gap-1.5"><Dot tone="yellow" />{t('status.late')}</span>
              <span className="inline-flex items-center gap-1.5"><Dot tone="red" />{t('status.absent')}</span>
            </div>
          </Card>
        </div>
      </div>
    </>
  )
}

export default function Dashboard() {
  return <RequireSite>{(site) => <Today site={site} />}</RequireSite>
}
