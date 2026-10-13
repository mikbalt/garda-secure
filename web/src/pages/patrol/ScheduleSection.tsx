import { useId } from 'react'
import { CalendarClock, Plus, Trash2 } from 'lucide-react'
import { Button } from '../../components/ui/Button'
import { Checkbox, Input } from '../../components/ui/Form'
import { useI18n, type Key } from '../../i18n/context'
import { ERROR_CODES } from '../../i18n/errors'
import { addDays, formatDay, formatTime, localDateOf, todayIn, weekdayName } from '../../lib/time'
import { MAX_SCHEDULES, parseClock, plannedPatrols, validateSchedule, type ScheduleError } from '../../lib/patrolSchedule'
import { Segmented } from './kit'
import { describeSchedule, newDraft, toSchedule, type ScheduleDraft } from './scheduleDrafts'

const DAY_PRESETS: { key: Key; days: number[] }[] = [
  { key: 'p2.sch.presetAll', days: [1, 2, 3, 4, 5, 6, 7] },
  { key: 'p2.sch.presetWork', days: [1, 2, 3, 4, 5] },
  { key: 'p2.sch.presetWeekend', days: [6, 7] },
]

function WindowCard({
  d,
  index,
  error,
  onChange,
  onRemove,
}: {
  d: ScheduleDraft
  index: number
  error?: ScheduleError
  onChange: (patch: Partial<ScheduleDraft>) => void
  onRemove: () => void
}) {
  const { t, lang } = useI18n()
  const id = useId()
  const s = toSchedule(d)
  const own = validateSchedule(s)
  const err = error ?? own
  const bad = (f: ScheduleError['field']) => (err?.field === f ? true : undefined)
  const toggle = (day: number) => onChange({ weekdays: d.weekdays.includes(day) ? d.weekdays.filter((x) => x !== day) : [...d.weekdays, day].sort((a, b) => a - b) })
  const label = t('p2.sch.window', { n: index + 1 })

  return (
    <li className={`rounded-xl border bg-white p-3.5 shadow-sm ${err ? 'border-red-300' : 'border-gray-200'} ${d.active ? '' : 'opacity-80'}`}>
      <fieldset>
        <legend className="sr-only">{label}</legend>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <p className="text-sm font-semibold text-gray-900" aria-hidden="true">
            {label}
          </p>
          <div className="flex items-center gap-2">
            <Checkbox label={t('common.active')} checked={d.active} onChange={(e) => onChange({ active: e.target.checked })} />
            <Button size="sm" variant="ghost" className="h-8 w-8 !px-0 text-red-600 hover:bg-red-50 hover:text-red-700" onClick={onRemove} aria-label={`${t('p2.sch.remove')} — ${label}`} title={t('p2.sch.remove')}>
              <Trash2 className="h-4 w-4" aria-hidden="true" />
            </Button>
          </div>
        </div>

        <div className="mt-3">
          <span id={`${id}-days`} className="mb-1.5 block text-xs font-medium text-gray-600">
            {t('p2.sch.days')}
          </span>
          <div className="flex flex-wrap items-center gap-1.5">
            <div role="group" aria-labelledby={`${id}-days`} className="inline-flex flex-wrap gap-1">
              {[1, 2, 3, 4, 5, 6, 7].map((day) => {
                const on = d.weekdays.includes(day)
                return (
                  <button
                    key={day}
                    type="button"
                    aria-pressed={on}
                    onClick={() => toggle(day)}
                    className={`h-8 min-w-[2.75rem] rounded-lg px-2 text-xs font-semibold ring-1 ring-inset transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 ${
                      on ? 'bg-primary-600 text-white ring-primary-600' : 'bg-white text-gray-700 ring-gray-300 hover:bg-gray-50'
                    } ${bad('weekdays') ? '!ring-red-500' : ''}`}
                  >
                    {weekdayName(day, lang)}
                  </button>
                )
              })}
            </div>
            <span className="mx-1 hidden h-5 w-px bg-gray-200 sm:inline-block" aria-hidden="true" />
            {DAY_PRESETS.map((p) => (
              <button key={p.key} type="button" onClick={() => onChange({ weekdays: p.days })} className="rounded-md px-2 py-1 text-xs font-medium text-primary-700 hover:bg-primary-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500">
                {t(p.key)}
              </button>
            ))}
          </div>
        </div>

        <div className="mt-3 flex flex-wrap items-end gap-3">
          <Segmented<'repeat' | 'once'>
            label={t('p2.sch.mode')}
            size="sm"
            value={d.mode}
            onChange={(mode) => onChange({ mode })}
            options={[
              { value: 'repeat', label: t('p2.sch.repeat') },
              { value: 'once', label: t('p2.sch.once') },
            ]}
          />
          <div>
            <label htmlFor={`${id}-start`} className="mb-1.5 block text-xs font-medium text-gray-600">
              {d.mode === 'repeat' ? t('p2.sch.from') : t('p2.sch.at')}
            </label>
            <Input id={`${id}-start`} type="time" value={d.start} onChange={(e) => onChange({ start: e.target.value })} aria-invalid={bad('start_time')} className="!h-9 w-32 !py-1" />
          </div>
          {d.mode === 'repeat' && (
            <>
              <div>
                <label htmlFor={`${id}-end`} className="mb-1.5 block text-xs font-medium text-gray-600">
                  {t('p2.sch.until')}
                </label>
                <Input id={`${id}-end`} type="time" value={d.end} onChange={(e) => onChange({ end: e.target.value })} aria-invalid={bad('end_time')} className="!h-9 w-32 !py-1" />
              </div>
              <div>
                <label htmlFor={`${id}-every`} className="mb-1.5 block text-xs font-medium text-gray-600">
                  {t('p2.sch.every')}
                </label>
                <div className="relative">
                  <Input
                    id={`${id}-every`}
                    type="number"
                    inputMode="numeric"
                    min={15}
                    max={720}
                    step={15}
                    value={d.every}
                    onChange={(e) => onChange({ every: e.target.value })}
                    aria-invalid={bad('every_minutes')}
                    className="!h-9 w-28 !py-1 pr-11 tabular-nums"
                  />
                  <span className="pointer-events-none absolute inset-y-0 right-3 flex items-center text-xs text-gray-500">{t('ops.pat.minShort')}</span>
                </div>
              </div>
            </>
          )}
          <div>
            <label htmlFor={`${id}-grace`} className="mb-1.5 block text-xs font-medium text-gray-600">
              {t('p2.sch.grace')}
            </label>
            <div className="relative">
              <Input
                id={`${id}-grace`}
                type="number"
                inputMode="numeric"
                min={0}
                max={180}
                value={d.grace}
                onChange={(e) => onChange({ grace: e.target.value })}
                aria-invalid={bad('grace_minutes')}
                className="!h-9 w-24 !py-1 pr-11 tabular-nums"
              />
              <span className="pointer-events-none absolute inset-y-0 right-3 flex items-center text-xs text-gray-500">{t('ops.pat.minShort')}</span>
            </div>
          </div>
        </div>

        <p className={`mt-3 text-sm ${err ? 'font-medium text-red-700' : 'text-gray-700'}`}>
          {err ? ERROR_CODES[err.code]?.[lang] : describeSchedule(s, t, lang)}
        </p>
      </fieldset>
    </li>
  )
}

/**
 * Route editor section "Schedule" (D-038): weekly windows, each either
 * repeating (every N minutes between start and end) or once, with a grace
 * period; a human preview per window and the next planned patrol times.
 */
export function ScheduleSection({
  tz,
  drafts,
  onChange,
  serverError,
  loading,
}: {
  tz: string
  drafts: ScheduleDraft[]
  onChange: (next: ScheduleDraft[]) => void
  /** Error of the last save, by window index. */
  serverError?: { index: number; error: ScheduleError } | null
  loading?: boolean
}) {
  const { t, lang } = useI18n()
  const valid = drafts.map(toSchedule).filter((s) => !validateSchedule(s))
  const upcoming = plannedPatrols(valid, tz, Date.now(), 36 * 3600_000, 6)
  const today = todayIn(tz)
  const dayText = (iso: string) => {
    const d = localDateOf(iso, tz)
    return d === today ? t('ops.day.today') : d === addDays(today, 1) ? t('p2.sch.tomorrow') : formatDay(d, lang, false)
  }
  const update = (i: number, patch: Partial<ScheduleDraft>) => onChange(drafts.map((d, j) => (j === i ? { ...d, ...patch } : d)))
  const hasStartIssue = drafts.some((d) => parseClock(d.start) === null)

  return (
    <fieldset>
      <legend className="flex items-center gap-2 text-sm font-semibold text-gray-900">
        <CalendarClock className="h-4 w-4 text-gray-500" aria-hidden="true" />
        {t('p2.sch.title')}
      </legend>
      <p className="mb-3 mt-0.5 text-sm text-gray-500">{t('p2.sch.intro')}</p>
      {loading ? (
        <div className="h-24 animate-pulse rounded-xl bg-gray-100" aria-hidden="true" />
      ) : (
        <>
          {drafts.length === 0 ? (
            <div className="rounded-xl border border-dashed border-gray-300 bg-gray-50/60 px-4 py-5 text-center text-sm text-gray-600">{t('p2.sch.empty')}</div>
          ) : (
            <ol className="space-y-2">
              {drafts.map((d, i) => (
                <WindowCard
                  key={d.key}
                  d={d}
                  index={i}
                  error={serverError && serverError.index === i ? serverError.error : undefined}
                  onChange={(patch) => update(i, patch)}
                  onRemove={() => onChange(drafts.filter((_, j) => j !== i))}
                />
              ))}
            </ol>
          )}
          <div className="mt-3 flex flex-wrap items-center justify-between gap-3">
            <Button variant="soft" onClick={() => onChange([...drafts, newDraft()])} disabled={drafts.length >= MAX_SCHEDULES}>
              <Plus className="h-4 w-4" aria-hidden="true" />
              {t('p2.sch.add')}
            </Button>
            {drafts.length >= MAX_SCHEDULES && <p className="text-xs text-gray-500">{ERROR_CODES.too_many_schedules[lang]}</p>}
          </div>
          {valid.some((s) => s.active) && !hasStartIssue && (
            <div className="mt-4 rounded-xl bg-gray-50 px-3.5 py-3">
              <p className="text-xs font-semibold text-gray-700">{t('p2.sch.upcoming')}</p>
              {upcoming.length === 0 ? (
                <p className="mt-1 text-xs text-gray-500">{t('p2.sch.upcomingNone')}</p>
              ) : (
                <ul className="mt-1.5 flex flex-wrap gap-1.5" aria-label={t('p2.sch.upcoming')}>
                  {upcoming.map((p) => (
                    <li key={p.due_at} className="rounded-full bg-white px-2.5 py-1 text-xs tabular-nums text-gray-700 ring-1 ring-inset ring-gray-200">
                      {dayText(p.due_at)} {formatTime(p.due_at, tz)}
                    </li>
                  ))}
                </ul>
              )}
              <p className="mt-1.5 text-xs text-gray-500">{t('p2.sch.upcomingHint')}</p>
            </div>
          )}
        </>
      )}
    </fieldset>
  )
}
