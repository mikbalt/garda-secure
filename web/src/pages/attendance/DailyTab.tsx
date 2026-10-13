import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { CalendarClock, CalendarX2, Flag, History, Info, Moon, Pencil, Search } from 'lucide-react'
import { apiGet } from '../../api/client'
import { DownloadCsvButton } from '../../components/DownloadButton'
import { FlagBadges } from '../../components/FlagBadges'
import { TableSkeleton } from '../../components/LoadingSkeleton'
import { MediaPhotoButton } from '../../components/MediaPhoto'
import { Badge, StatusBadge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Input } from '../../components/ui/Form'
import { EmptyState, ErrorState } from '../../components/ui/States'
import { buttonClass } from '../../components/ui/styles'
import { Table, type Column } from '../../components/ui/Table'
import { useAuth } from '../../hooks/useAuth'
import { useI18n } from '../../i18n/context'
import { addDays, formatNumber, formatTime, localDateOf } from '../../lib/time'
import { FilterChips, PeriodStepper, PersonCell, type ChipItem } from '../guards/PeopleKit'
import { formatLongDay } from '../guards/peopleUtil'
import { OverrideModal } from './OverrideModal'
import { HistoryDrawer } from './HistoryDrawer'
import type { DailyAttendance, DailyAttendanceRow, Site } from '../../types'

type Filter = 'all' | 'present' | 'late' | 'absent' | 'pending' | 'review' | 'flagged'

interface Props {
  site: Site
  date: string
  today: string
  onDateChange: (date: string) => void
}

export function DailyTab({ site, date, today, onDateChange }: Props) {
  const { t, lang } = useI18n()
  const { can } = useAuth()
  const tz = site.timezone
  const [filter, setFilter] = useState<Filter>('all')
  const [editing, setEditing] = useState<DailyAttendanceRow | null>(null)
  const [historyOf, setHistoryOf] = useState<DailyAttendanceRow | null>(null)
  const canHistory = can('viewAttendanceHistory')
  const canOverride = can('overrideAttendance')

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['attendance-daily', site.id, date],
    queryFn: () => apiGet<DailyAttendance>('/attendance/daily', { site_id: site.id, date }),
    enabled: !!date,
    refetchInterval: date === today ? 60_000 : false,
  })

  const all = useMemo(() => data?.rows ?? [], [data])
  const counts = useMemo(
    () => ({
      all: all.length,
      present: all.filter((r) => r.status === 'present').length,
      late: all.filter((r) => r.status === 'late').length,
      absent: all.filter((r) => r.status === 'absent').length,
      pending: all.filter((r) => r.status === 'pending').length,
      review: all.filter((r) => r.needs_review).length,
      flagged: all.filter((r) => (r.flags?.length ?? 0) > 0).length,
    }),
    [all]
  )
  const rows =
    filter === 'all'
      ? all
      : filter === 'review'
        ? all.filter((r) => r.needs_review)
        : filter === 'flagged'
          ? all.filter((r) => (r.flags?.length ?? 0) > 0)
          : all.filter((r) => r.status === filter)

  const chips: ChipItem<Filter>[] = [
    { key: 'all', label: t('ppl.att.f.all'), count: counts.all },
    { key: 'present', label: t('status.present'), count: counts.present, dot: 'bg-emerald-500' },
    { key: 'late', label: t('status.late'), count: counts.late, dot: 'bg-amber-500' },
    { key: 'absent', label: t('status.absent'), count: counts.absent, dot: 'bg-red-500' },
    { key: 'pending', label: t('ppl.att.notIn'), count: counts.pending, dot: 'bg-gray-400' },
    { key: 'review', label: t('att.needsReview'), count: counts.review, dot: 'bg-violet-500' },
    ...(counts.flagged > 0 ? [{ key: 'flagged' as const, label: t('flag.filter'), count: counts.flagged, dot: 'bg-amber-500' }] : []),
  ]

  const now = Date.now()

  const statusCell = (r: DailyAttendanceRow) => {
    let main
    if (r.status === 'pending') {
      const started = Date.parse(r.shift_start) <= now
      main = (
        <div>
          <Badge tone={started ? 'orange' : 'gray'} dot>
            {t('ppl.att.notIn')}
          </Badge>
          <p className="mt-1 text-xs text-gray-600">
            {started ? t('ppl.att.startedNoClockIn') : t('ppl.att.startsAt', { time: formatTime(r.shift_start, tz) })}
          </p>
        </div>
      )
    } else {
      main = <StatusBadge status={r.status} />
    }
    return (
      <div className="flex flex-wrap items-start gap-1.5">
        {main}
        {r.override_reason && (
          <Badge tone="blue">
            <Info className="h-3 w-3" aria-hidden="true" />
            <span title={t('att.overridden', { reason: r.override_reason })}>{t('ppl.att.corrected')}</span>
            <span className="sr-only">{t('att.overridden', { reason: r.override_reason })}</span>
          </Badge>
        )}
        {r.override_reason && canHistory && (
          <Button size="sm" variant="ghost" className="!h-6 !px-2" onClick={() => setHistoryOf(r)} aria-label={`${t('acc.hist.open')} — ${r.guard_name}`}>
            <History className="h-3.5 w-3.5" aria-hidden="true" />
            {t('acc.hist.open')}
          </Button>
        )}
        {r.needs_review && (
          <Badge tone="purple">
            <Flag className="h-3 w-3" aria-hidden="true" />
            {t('att.needsReview')}
          </Badge>
        )}
        <FlagBadges flags={r.flags} />
      </div>
    )
  }

  const columns: Column<DailyAttendanceRow>[] = [
    {
      key: 'guard',
      header: t('common.guard'),
      cell: (r) => <PersonCell name={r.guard_name} sub={r.employee_id} />,
    },
    {
      key: 'shift',
      header: t('att.shift'),
      cell: (r) => {
        const overnight = localDateOf(r.shift_end, tz) !== localDateOf(r.shift_start, tz)
        return (
          <span className="inline-flex items-center gap-1 whitespace-nowrap tabular-nums text-gray-700">
            {formatTime(r.shift_start, tz)}–{formatTime(r.shift_end, tz)}
            {overnight && (
              <span title={t('sch.endsNextDay')} className="inline-flex items-center text-indigo-500">
                <Moon className="h-3.5 w-3.5" aria-hidden="true" />
                <span className="sr-only">{t('sch.endsNextDay')}</span>
              </span>
            )}
          </span>
        )
      },
    },
    { key: 'status', header: t('common.status'), cell: statusCell },
    {
      key: 'in',
      header: t('att.clockIn'),
      cell: (r) => (
        <span className="inline-flex items-center gap-1.5 whitespace-nowrap tabular-nums">
          <span className={r.clock_in_at ? 'font-medium text-gray-900' : 'text-gray-500'}>{formatTime(r.clock_in_at, tz)}</span>
          {r.late_minutes > 0 && (
            <span title={t('ppl.att.lateTitle', { n: r.late_minutes })}>
              <Badge tone="yellow">{t('ppl.att.lateBy', { n: r.late_minutes })}</Badge>
              <span className="sr-only">{t('ppl.att.lateTitle', { n: r.late_minutes })}</span>
            </span>
          )}
        </span>
      ),
    },
    {
      key: 'out',
      header: t('att.clockOut'),
      cell: (r) => {
        const onDuty = r.clock_in_at && !r.clock_out_at && now < Date.parse(r.shift_end)
        return (
          <span className="inline-flex items-center gap-1.5 whitespace-nowrap tabular-nums">
            {onDuty ? (
              <span className="text-xs font-medium text-emerald-700">{t('ppl.att.onDuty')}</span>
            ) : (
              <span className={r.clock_out_at ? 'font-medium text-gray-900' : 'text-gray-500'}>{formatTime(r.clock_out_at, tz)}</span>
            )}
            {r.early_out_minutes > 0 && (
              <span title={t('ppl.att.earlyTitle', { n: r.early_out_minutes })}>
                <Badge tone="yellow">{t('ppl.att.earlyBy', { n: r.early_out_minutes })}</Badge>
                <span className="sr-only">{t('ppl.att.earlyTitle', { n: r.early_out_minutes })}</span>
              </span>
            )}
          </span>
        )
      },
    },
    {
      key: 'hours',
      header: t('ppl.att.hoursWorked'),
      numeric: true,
      cell: (r) => (r.hours_worked ? <span className="text-gray-900">{formatNumber(r.hours_worked, lang, 1)}</span> : <span className="text-gray-500">—</span>),
    },
  ]
  // Selfies are HR evidence: client-readonly may not open them (media_forbidden).
  if (can('viewSelfies')) {
    columns.push({
      key: 'selfie',
      header: t('att.selfie'),
      className: '[&_span]:whitespace-nowrap',
      cell: (r) => <MediaPhotoButton objectKey={r.selfie_object_key} title={`${t('att.selfie')}: ${r.guard_name}`} />,
    })
  }
  if (canOverride) {
    columns.push({
      key: 'actions',
      header: <span className="sr-only">{t('common.actions')}</span>,
      className: 'text-right',
      cell: (r) => (
        <Button size="sm" variant="secondary" onClick={() => setEditing(r)} aria-label={`${t('att.override')} — ${r.guard_name}`}>
          <Pencil className="h-3.5 w-3.5" aria-hidden="true" />
          {t('att.override')}
        </Button>
      ),
    })
  }

  let body
  if (isLoading) body = <TableSkeleton />
  else if (error) body = <ErrorState error={error} onRetry={() => refetch()} />
  else if (all.length === 0)
    body = (
      <EmptyState icon={CalendarX2} title={t('ppl.att.emptyTitle')} message={t('ppl.att.emptyBody')}>
        {can('viewSchedules') && (
          <Link to="/schedules" className={buttonClass('secondary')}>
            <CalendarClock className="h-4 w-4" aria-hidden="true" />
            {t('ppl.att.emptyCta')}
          </Link>
        )}
      </EmptyState>
    )
  else if (rows.length === 0)
    body = (
      <EmptyState icon={Search} title={t('ppl.att.noMatchTitle')} message={t('ppl.att.noMatchBody')}>
        <Button variant="secondary" onClick={() => setFilter('all')}>
          {t('ppl.att.showAll')}
        </Button>
      </EmptyState>
    )
  else
    body = (
      // `relative` + clip keeps screen-reader-only text inside the scrolling table
      // from widening the page on phones.
      <div className="relative -m-1 overflow-hidden p-1">
        <Table
          columns={columns}
          rows={rows}
          rowKey={(r) => r.shift_id}
          caption={`${t('att.title')} — ${formatLongDay(date, lang)}`}
          rowClassName={(r) => (r.status === 'absent' ? 'bg-red-50/30' : '')}
        />
      </div>
    )

  return (
    <>
      <div className="no-print mb-4 rounded-2xl border border-gray-200/80 bg-white shadow-card">
        <div className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
          <PeriodStepper
            label={formatLongDay(date, lang)}
            sublabel={
              <>
                {date === today && <span className="font-medium text-primary-700">{t('ppl.att.today')} · </span>}
                {data ? t('ppl.att.shiftsCount', { n: all.length }) : '…'} · {t('ppl.att.tzNote', { tz })}
              </>
            }
            onPrev={() => onDateChange(addDays(date, -1))}
            onNext={() => onDateChange(addDays(date, 1))}
            onToday={() => onDateChange(today)}
            isCurrent={date === today}
            todayLabel={t('ppl.att.today')}
            prevLabel={t('ppl.att.prevDay')}
            nextLabel={t('ppl.att.nextDay')}
          />
          <div className="flex flex-wrap items-center gap-2">
            <label htmlFor="att-date" className="sr-only">
              {t('ppl.att.pickDate')}
            </label>
            <Input id="att-date" type="date" value={date} onChange={(e) => e.target.value && onDateChange(e.target.value)} className="!w-auto" />
            <DownloadCsvButton path="/reports/attendance/daily" params={{ site_id: site.id, date }} filename={`attendance-${date}.csv`} />
          </div>
        </div>
        {all.length > 0 && (
          <div className="border-t border-gray-100 px-4 py-3">
            <FilterChips label={t('ppl.att.filterLabel')} items={chips} active={filter} onChange={setFilter} />
          </div>
        )}
      </div>
      {body}
      {editing && <OverrideModal site={site} row={editing} onClose={() => setEditing(null)} />}
      {historyOf && <HistoryDrawer site={site} row={historyOf} onClose={() => setHistoryOf(null)} />}
    </>
  )
}
