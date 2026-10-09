import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CalendarRange, Moon, Pencil, Plus, Repeat, Trash2 } from 'lucide-react'
import apiClient, { apiGet } from '../../api/client'
import { CardSkeleton } from '../../components/LoadingSkeleton'
import { Badge, StatusBadge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { FormError } from '../../components/ui/Form'
import { EmptyState, ErrorState, Notice } from '../../components/ui/States'
import { useToast } from '../../components/toast/context'
import { useGuards } from '../../hooks/useGuards'
import { useI18n } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import { formatDay, formatNumber, todayIn, weekdayName } from '../../lib/time'
import { Avatar, ConfirmDialog } from '../guards/PeopleKit'
import { periodChip, periodDot, periodOf } from '../guards/peopleUtil'
import type { ShiftTemplate, Site } from '../../types'

const DAYS = [1, 2, 3, 4, 5, 6, 7] as const

function durationHours(start: string, end: string): number {
  const toMin = (s: string) => Number(s.slice(0, 2)) * 60 + Number(s.slice(3, 5))
  let d = toMin(end) - toMin(start)
  if (d <= 0) d += 24 * 60
  return Math.round((d / 60) * 2) / 2
}

/** Seven small day squares (S S R K J S M), filled on the pattern's days. */
export function WeekdayDots({ weekdays }: { weekdays: number[] }) {
  const { t, lang } = useI18n()
  const label = [...weekdays]
    .sort()
    .map((d) => weekdayName(d, lang))
    .join(', ')
  return (
    <div className="flex gap-1" role="img" aria-label={`${t('sch.weekdays')}: ${label}`}>
      {DAYS.map((d) => {
        const on = weekdays.includes(d)
        return (
          <span
            key={d}
            aria-hidden="true"
            className={`flex h-6 w-6 items-center justify-center rounded-md text-[11px] font-semibold ${
              on ? 'bg-primary-600 text-white' : 'bg-gray-100 text-gray-400'
            }`}
          >
            {t(`ppl.wd.${d}`)}
          </span>
        )
      })}
    </div>
  )
}

interface Props {
  site: Site
  canEdit: boolean
  onNew: () => void
  onEdit: (tpl: ShiftTemplate) => void
}

export function TemplatesTab({ site, canEdit, onNew, onEdit }: Props) {
  const { t, lang } = useI18n()
  const qc = useQueryClient()
  const toast = useToast()
  const { guards, guardName } = useGuards(site.id)
  const [deleting, setDeleting] = useState<ShiftTemplate | null>(null)
  const today = todayIn(site.timezone)

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['shift-templates', site.id],
    queryFn: () => apiGet<ShiftTemplate[]>('/shift-templates', { site_id: site.id }),
  })

  const del = useMutation({
    mutationFn: (id: string) => apiClient.delete(`/shift-templates/${id}`),
    onSuccess: () => {
      toast.success(t('sch.templateDeleted'))
      qc.invalidateQueries({ queryKey: ['shift-templates', site.id] })
      qc.invalidateQueries({ queryKey: ['shifts', site.id] })
      setDeleting(null)
    },
  })

  const sorted = useMemo(
    () =>
      [...(data ?? [])].sort(
        (a, b) => Number(b.active) - Number(a.active) || guardName(a.guard_id).localeCompare(guardName(b.guard_id)) || a.start_time.localeCompare(b.start_time)
      ),
    // guardName changes identity every render; guards captures its content
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [data, guards]
  )

  const empId = (id: string) => guards.find((g) => g.id === id)?.employee_id

  let body
  if (isLoading)
    body = (
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
        {Array.from({ length: 3 }).map((_, i) => (
          <CardSkeleton key={i} />
        ))}
      </div>
    )
  else if (error) body = <ErrorState error={error} onRetry={() => refetch()} />
  else if (sorted.length === 0)
    body = (
      <EmptyState icon={Repeat} title={t('ppl.sch.patternsEmptyTitle')} message={canEdit ? t('ppl.sch.patternsEmptyBody') : t('ppl.sch.patternsEmptyRo')}>
        {canEdit && (
          <Button onClick={onNew}>
            <Plus className="h-4 w-4" aria-hidden="true" />
            {t('ppl.sch.newPattern')}
          </Button>
        )}
      </EmptyState>
    )
  else
    body = (
      <ul className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
        {sorted.map((tpl) => {
          const name = guardName(tpl.guard_id)
          const period = periodOf(tpl.start_time)
          const overnight = tpl.end_time <= tpl.start_time
          const expired = !!tpl.valid_until && tpl.valid_until < today
          const upcoming = tpl.valid_from > today
          return (
            <li
              key={tpl.id}
              className={`flex flex-col rounded-2xl border border-gray-200/80 bg-white shadow-card transition-shadow hover:shadow-raised ${tpl.active && !expired ? '' : 'bg-gray-50/60'}`}
            >
              <div className="flex items-start justify-between gap-3 px-5 pt-5">
                <div className="flex min-w-0 items-center gap-3">
                  <Avatar name={name} muted={!tpl.active} />
                  <div className="min-w-0">
                    <p className="truncate text-[15px] font-semibold text-gray-900">{name}</p>
                    <p className="truncate text-xs text-gray-500">{empId(tpl.guard_id) ?? '—'}</p>
                  </div>
                </div>
                {expired ? <Badge>{t('ppl.sch.expired')}</Badge> : upcoming && tpl.active ? <Badge tone="blue">{t('ppl.sch.upcoming')}</Badge> : <StatusBadge status={tpl.active ? 'active' : 'inactive'} />}
              </div>

              <div className="flex-1 space-y-3 px-5 py-4">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-xl font-semibold tabular-nums tracking-tight text-gray-900">
                    {tpl.start_time}–{tpl.end_time}
                  </span>
                  <span className={`inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset ${periodChip[period]}`}>
                    <span className={`h-1.5 w-1.5 rounded-full ${periodDot[period]}`} aria-hidden="true" />
                    {t(`ppl.sch.period.${period}`)}
                  </span>
                </div>
                <p className="flex flex-wrap items-center gap-x-2 text-xs text-gray-500">
                  <span>{t('ppl.sch.hoursShift', { h: formatNumber(durationHours(tpl.start_time, tpl.end_time), lang, 0) })}</span>
                  {overnight && (
                    <span className="inline-flex items-center gap-1 font-medium text-indigo-700">
                      <Moon className="h-3 w-3" aria-hidden="true" />
                      {t('ppl.sch.endsTomorrow')}
                    </span>
                  )}
                  <span>· {t('ppl.sch.perWeek', { n: tpl.weekdays.length })}</span>
                </p>
                <WeekdayDots weekdays={tpl.weekdays} />
                <p className="flex items-center gap-1.5 text-xs text-gray-500">
                  <CalendarRange className="h-3.5 w-3.5 text-gray-400" aria-hidden="true" />
                  {t('ppl.sch.validity', {
                    from: formatDay(tpl.valid_from, lang, false),
                    to: tpl.valid_until ? formatDay(tpl.valid_until, lang, false) : t('sch.openEnded'),
                  })}
                </p>
              </div>

              {canEdit && (
                <div className="flex items-center justify-end gap-1 border-t border-gray-100 px-3 py-2">
                  <Button size="sm" variant="ghost" onClick={() => onEdit(tpl)} aria-label={`${t('common.edit')} — ${name} ${tpl.start_time}`}>
                    <Pencil className="h-3.5 w-3.5" aria-hidden="true" />
                    {t('common.edit')}
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    className="text-red-700 hover:bg-red-50 hover:text-red-800"
                    onClick={() => {
                      del.reset()
                      setDeleting(tpl)
                    }}
                    aria-label={`${t('common.delete')} — ${name} ${tpl.start_time}`}
                  >
                    <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
                    {t('common.delete')}
                  </Button>
                </div>
              )}
            </li>
          )
        })}
      </ul>
    )

  return (
    <>
      <Notice>{t('ppl.sch.patternsNotice')}</Notice>
      {body}
      {deleting && (
        <ConfirmDialog
          title={t('ppl.sch.deletePatternTitle')}
          message={t('ppl.sch.deletePatternBody', { guard: guardName(deleting.guard_id), time: `${deleting.start_time}–${deleting.end_time}` })}
          confirmLabel={t('ppl.sch.yesDelete')}
          loading={del.isPending}
          error={<FormError message={del.error ? errorText(del.error, lang) : null} />}
          onConfirm={() => del.mutate(deleting.id)}
          onClose={() => setDeleting(null)}
        />
      )}
    </>
  )
}
