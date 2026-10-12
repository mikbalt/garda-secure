import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { CalendarPlus, Moon, Plus, UserPlus, Users } from 'lucide-react'
import { apiGet } from '../../api/client'
import { TableSkeleton } from '../../components/LoadingSkeleton'
import { Button } from '../../components/ui/Button'
import { EmptyState, ErrorState } from '../../components/ui/States'
import { buttonClass } from '../../components/ui/styles'
import { useAuth } from '../../hooks/useAuth'
import { useGuards } from '../../hooks/useGuards'
import { useI18n } from '../../i18n/context'
import { addDays, formatNumber, formatTime, localDateOf, localTimeOf, startOfWeek, todayIn } from '../../lib/time'
import { Avatar, PeriodStepper } from '../guards/PeopleKit'
import { dayHeader, formatDayRange, formatLongDay, hoursBetween, periodChip, periodDashed, periodOf, type DayPeriod } from '../guards/peopleUtil'
import type { Guard, Shift, Site } from '../../types'

interface Props {
  site: Site
  canEdit: boolean
  onAdd: (guardId: string, date: string) => void
  onEdit: (shift: Shift) => void
  onNewPattern: () => void
}

const PERIODS: DayPeriod[] = ['morning', 'afternoon', 'night']

/** Weekly roster: guards × 7 days, shift chips coloured by time of day. */
export function RosterTab({ site, canEdit, onAdd, onEdit, onNewPattern }: Props) {
  const { t, lang } = useI18n()
  const { can } = useAuth()
  const tz = site.timezone
  const today = todayIn(tz)
  const thisWeek = startOfWeek(today)
  const [from, setFrom] = useState(thisWeek)
  const to = addDays(from, 6)
  const days = useMemo(() => Array.from({ length: 7 }, (_, i) => addDays(from, i)), [from])

  const guardsQ = useGuards(site.id)
  const shiftsQ = useQuery({
    queryKey: ['shifts', site.id, from, to],
    queryFn: () => apiGet<Shift[]>('/shifts', { site_id: site.id, from, to }),
  })

  const { rows, cells, perDay, totalHours, idle } = useMemo(() => {
    const shifts = shiftsQ.data ?? []
    const cells = new Map<string, Shift[]>()
    const perDay = new Map<string, number>()
    let totalHours = 0
    for (const s of [...shifts].sort((a, b) => a.start_time.localeCompare(b.start_time))) {
      const day = localDateOf(s.start_time, tz)
      const key = `${s.guard_id}|${day}`
      cells.set(key, [...(cells.get(key) ?? []), s])
      perDay.set(day, (perDay.get(day) ?? 0) + 1)
      totalHours += hoursBetween(s.start_time, s.end_time)
    }
    const withShifts = new Set(shifts.map((s) => s.guard_id))
    const known = new Map(guardsQ.guards.map((g) => [g.id, g]))
    const rows: Pick<Guard, 'id' | 'name' | 'employee_id' | 'status'>[] = guardsQ.guards.filter((g) => g.status === 'active' || withShifts.has(g.id))
    for (const id of withShifts) if (!known.has(id)) rows.push({ id, name: '—', status: 'inactive' })
    rows.sort((a, b) => a.name.localeCompare(b.name))
    const idle = rows.filter((g) => g.status === 'active' && !withShifts.has(g.id)).length
    return { rows, cells, perDay, totalHours, idle }
  }, [shiftsQ.data, guardsQ.guards, tz])

  const shiftCount = shiftsQ.data?.length ?? 0
  const loading = guardsQ.isLoading || shiftsQ.isLoading
  const error = guardsQ.error ?? shiftsQ.error

  const stepper = (
    <PeriodStepper
      label={formatDayRange(from, to, lang)}
      sublabel={
        shiftsQ.data ? (
          <>
            {from === thisWeek && <span className="font-medium text-primary-600">{t('ppl.sch.thisWeek')} · </span>}
            {t('ppl.sch.summary', { n: shiftCount, h: formatNumber(totalHours, lang, 0) })}
            {idle > 0 && <span className="text-amber-700"> · {t('ppl.sch.idle', { n: idle })}</span>}
          </>
        ) : (
          '…'
        )
      }
      onPrev={() => setFrom(addDays(from, -7))}
      onNext={() => setFrom(addDays(from, 7))}
      onToday={() => setFrom(thisWeek)}
      isCurrent={from === thisWeek}
      todayLabel={t('ppl.sch.thisWeek')}
      prevLabel={t('common.prevWeek')}
      nextLabel={t('common.nextWeek')}
    />
  )

  const legend = (
    <ul className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-gray-600" aria-label={t('ppl.sch.legend')}>
      {PERIODS.map((p) => (
        <li key={p} className="inline-flex items-center gap-1.5">
          <span className={`h-3 w-4 rounded ring-1 ring-inset ${periodChip[p]}`} aria-hidden="true" />
          {t(`ppl.sch.period.${p}`)} <span className="text-gray-500">{t(`ppl.sch.periodRange.${p}`)}</span>
        </li>
      ))}
      <li className="inline-flex items-center gap-1.5">
        <span className="h-3 w-4 rounded border border-dashed border-gray-400 bg-white" aria-hidden="true" />
        {t('sch.oneOff')}
      </li>
      <li className="inline-flex items-center gap-1.5">
        <Moon className="h-3 w-3 text-indigo-500" aria-hidden="true" />
        {t('ppl.sch.endsTomorrow')}
      </li>
    </ul>
  )

  let body
  if (loading) body = <TableSkeleton rows={6} />
  else if (error) body = <ErrorState error={error} onRetry={() => (guardsQ.error ? guardsQ.refetch() : shiftsQ.refetch())} />
  else if (rows.length === 0)
    body = (
      <EmptyState icon={Users} title={t('ppl.sch.noGuardsTitle')} message={t('ppl.sch.noGuardsBody')}>
        {can('editGuards') && (
          <Link to="/guards?new=1" className={buttonClass('primary')}>
            <UserPlus className="h-4 w-4" aria-hidden="true" />
            {t('ppl.grd.add')}
          </Link>
        )}
      </EmptyState>
    )
  else
    body = (
      <>
        {shiftCount === 0 && (
          <div className="mb-4 flex flex-wrap items-center gap-3 rounded-xl border border-primary-100 bg-primary-50/70 px-4 py-3 text-sm text-primary-900">
            <CalendarPlus className="h-4 w-4 shrink-0 text-primary-600" aria-hidden="true" />
            <p className="min-w-0 flex-1">{canEdit ? t('ppl.sch.emptyWeek') : t('ppl.sch.emptyWeekRo')}</p>
            {canEdit && (
              <Button size="sm" onClick={onNewPattern}>
                {t('ppl.sch.newPattern')}
              </Button>
            )}
          </div>
        )}
        <div
          tabIndex={0}
          role="region"
          aria-label={t('ppl.sch.tabRoster')}
          className="scroll-thin relative overflow-x-auto rounded-2xl border border-gray-200/80 bg-white shadow-card focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
        >
          <table className="w-full min-w-[900px] sm:min-w-[960px] table-fixed border-separate border-spacing-0 text-sm">
            <caption className="sr-only">
              {t('ppl.sch.tabRoster')} {formatDayRange(from, to, lang)}
            </caption>
            <colgroup>
              <col className="w-[150px] sm:w-[210px]" />
              {days.map((d) => (
                <col key={d} />
              ))}
            </colgroup>
            <thead>
              <tr>
                <th scope="col" className="sticky left-0 z-20 border-b border-r border-gray-200 bg-gray-50 px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-wider text-gray-500">
                  {t('common.guard')}
                </th>
                {days.map((d) => {
                  const [wd, dd] = dayHeader(d, lang)
                  const isToday = d === today
                  return (
                    <th
                      key={d}
                      scope="col"
                      aria-current={isToday ? 'date' : undefined}
                      className={`border-b border-gray-200 px-2 py-2 text-center font-normal ${isToday ? 'bg-primary-50' : 'bg-gray-50'}`}
                    >
                      <span className="sr-only">{formatLongDay(d, lang)}</span>
                      <span aria-hidden="true" className="flex flex-col items-center leading-tight">
                        <span className={`text-[11px] font-semibold uppercase tracking-wider ${isToday ? 'text-primary-700' : 'text-gray-500'}`}>{wd}</span>
                        <span
                          className={`mt-0.5 inline-flex h-7 w-7 items-center justify-center rounded-full text-sm font-semibold tabular-nums ${
                            isToday ? 'bg-primary-600 text-white' : d < today ? 'text-gray-500' : 'text-gray-900'
                          }`}
                        >
                          {dd}
                        </span>
                      </span>
                    </th>
                  )
                })}
              </tr>
            </thead>
            <tbody>
              {rows.map((g) => (
                <tr key={g.id} className="group/row">
                  <th scope="row" className="sticky left-0 z-10 border-b border-r border-gray-100 bg-white px-4 py-2.5 text-left font-normal group-hover/row:bg-gray-50">
                    <div className="flex min-w-0 items-center gap-2.5">
                      <Avatar name={g.name} size="sm" muted={g.status === 'inactive'} />
                      <div className="min-w-0">
                        <p className="truncate font-medium text-gray-900">{g.name}</p>
                        {g.employee_id && <p className="truncate text-xs text-gray-500">{g.employee_id}</p>}
                      </div>
                    </div>
                  </th>
                  {days.map((d) => {
                    const list = cells.get(`${g.id}|${d}`) ?? []
                    const isToday = d === today
                    const past = d < today
                    const canAdd = canEdit && !past && g.status === 'active'
                    return (
                      <td
                        key={d}
                        className={`group/cell h-14 border-b border-gray-100 p-1.5 align-middle ${isToday ? 'bg-primary-50/40' : past ? 'bg-gray-50/60' : ''}`}
                      >
                        <div className="flex h-full flex-col gap-1">
                          {list.map((s) => (
                            <ShiftChip key={s.id} shift={s} tz={tz} guard={g.name} day={formatLongDay(d, lang)} canEdit={canEdit} onEdit={onEdit} />
                          ))}
                          {canAdd && (
                            <button
                              type="button"
                              onClick={() => onAdd(g.id, d)}
                              aria-label={t('ppl.sch.addHere', { guard: g.name, day: formatLongDay(d, lang) })}
                              title={t('ppl.sch.addOneOffShort')}
                              className={`flex items-center justify-center rounded-md border border-dashed text-gray-500 transition focus:outline-none focus-visible:border-primary-400 focus-visible:text-primary-600 focus-visible:opacity-100 focus-visible:ring-2 focus-visible:ring-primary-500 hover:border-primary-300 hover:bg-primary-50 hover:text-primary-600 ${
                                list.length === 0
                                  ? 'min-h-[40px] flex-1 border-transparent opacity-0 group-hover/cell:border-gray-300 group-hover/cell:opacity-100'
                                  : 'h-6 border-transparent opacity-0 group-hover/cell:border-gray-300 group-hover/cell:opacity-100'
                              }`}
                            >
                              <Plus className="h-3.5 w-3.5" aria-hidden="true" />
                            </button>
                          )}
                        </div>
                      </td>
                    )
                  })}
                </tr>
              ))}
            </tbody>
            <tfoot>
              <tr>
                <th scope="row" className="sticky left-0 z-10 border-r border-gray-200 bg-gray-50 px-4 py-2 text-left text-xs font-medium text-gray-500">
                  {t('ppl.sch.perDay')}
                </th>
                {days.map((d) => (
                  <td key={d} className={`px-2 py-2 text-center text-xs tabular-nums ${d === today ? 'bg-primary-50 font-semibold text-primary-700' : 'bg-gray-50 text-gray-600'}`}>
                    {perDay.get(d) ?? 0}
                  </td>
                ))}
              </tr>
            </tfoot>
          </table>
        </div>
        {canEdit && <p className="mt-3 text-xs text-gray-500">{t('ppl.sch.rosterHint')}</p>}
      </>
    )

  return (
    <>
      <div className="no-print mb-4 flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-gray-200/80 bg-white px-4 py-3 shadow-card">
        {stepper}
        {legend}
      </div>
      {body}
    </>
  )
}

function ShiftChip({
  shift,
  tz,
  guard,
  day,
  canEdit,
  onEdit,
}: {
  shift: Shift
  tz: string
  guard: string
  day: string
  canEdit: boolean
  onEdit: (s: Shift) => void
}) {
  const { t } = useI18n()
  const start = localTimeOf(shift.start_time, tz)
  const period = periodOf(start)
  const overnight = localDateOf(shift.end_time, tz) !== localDateOf(shift.start_time, tz)
  const time = `${formatTime(shift.start_time, tz)}–${formatTime(shift.end_time, tz)}`
  const kind = shift.template_id ? t('ppl.sch.fromPattern') : t('sch.oneOff')
  const content = (
    <>
      <span className="truncate tabular-nums">{time}</span>
      {overnight && <Moon className="h-3 w-3 shrink-0 opacity-70" aria-hidden="true" />}
    </>
  )
  const style = shift.template_id ? `ring-1 ring-inset ${periodChip[period]}` : periodDashed[period]
  const cls = `flex w-full min-w-0 items-center justify-center gap-1 rounded-md px-1.5 py-1 text-xs font-semibold ${style}`
  const label = `${guard}, ${day}, ${time}${overnight ? ` (${t('sch.endsNextDay')})` : ''}, ${kind}`
  if (!canEdit) {
    return (
      <div className={cls.replace(/hover:\S+/g, '')} title={label}>
        <span className="sr-only">{label}</span>
        <span aria-hidden="true" className="flex min-w-0 items-center gap-1">
          {content}
        </span>
      </div>
    )
  }
  return (
    <button
      type="button"
      onClick={() => onEdit(shift)}
      title={label}
      aria-label={`${t('common.edit')}: ${label}`}
      className={`${cls} text-left transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500`}
    >
      {content}
    </button>
  )
}
