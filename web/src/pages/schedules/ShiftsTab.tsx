import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, Plus, Pencil, Trash2, Repeat } from 'lucide-react'
import apiClient, { apiGet } from '../../api/client'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Field, Input } from '../../components/ui/Form'
import { EmptyState, ErrorState, Toolbar } from '../../components/ui/States'
import { TableSkeleton } from '../../components/LoadingSkeleton'
import { useErrorToast, useToast } from '../../components/toast/context'
import { useGuards } from '../../hooks/useGuards'
import { useI18n } from '../../i18n/context'
import { addDays, daysBetween, formatDay, formatTime, localDateOf, startOfWeek, todayIn } from '../../lib/time'
import { ShiftModal } from './ShiftModal'
import type { Shift, Site } from '../../types'

export function ShiftsTab({ site, canEdit }: { site: Site; canEdit: boolean }) {
  const { t, lang } = useI18n()
  const tz = site.timezone
  const qc = useQueryClient()
  const toast = useToast()
  const onError = useErrorToast()
  const { guardName } = useGuards(site.id)
  const [from, setFrom] = useState(() => startOfWeek(todayIn(tz)))
  const [to, setTo] = useState(() => addDays(startOfWeek(todayIn(tz)), 6))
  const [editing, setEditing] = useState<Shift | 'new' | null>(null)

  const validRange = from && to && from <= to && daysBetween(from, to) <= 62
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['shifts', site.id, from, to],
    queryFn: () => apiGet<Shift[]>('/shifts', { site_id: site.id, from, to }),
    enabled: !!validRange,
  })

  const del = useMutation({
    mutationFn: (id: string) => apiClient.delete(`/shifts/${id}`),
    onSuccess: () => {
      toast.success(t('common.deleted'))
      qc.invalidateQueries({ queryKey: ['shifts', site.id] })
    },
    onError,
  })

  const byDay = useMemo(() => {
    const groups = new Map<string, Shift[]>()
    for (const s of [...(data ?? [])].sort((a, b) => a.start_time.localeCompare(b.start_time))) {
      const day = localDateOf(s.start_time, tz)
      groups.set(day, [...(groups.get(day) ?? []), s])
    }
    return Array.from(groups.entries())
  }, [data, tz])

  const shiftWeek = (n: number) => {
    setFrom(addDays(from, 7 * n))
    setTo(addDays(to, 7 * n))
  }

  return (
    <>
      <Toolbar>
        <Button variant="secondary" onClick={() => shiftWeek(-1)} aria-label={t('common.prevWeek')}>
          <ChevronLeft className="h-4 w-4" aria-hidden="true" />
        </Button>
        <Field label={t('common.from')}>
          <Input type="date" value={from} onChange={(e) => setFrom(e.target.value)} />
        </Field>
        <Field label={t('common.to')}>
          <Input type="date" value={to} onChange={(e) => setTo(e.target.value)} />
        </Field>
        <Button variant="secondary" onClick={() => shiftWeek(1)} aria-label={t('common.nextWeek')}>
          <ChevronRight className="h-4 w-4" aria-hidden="true" />
        </Button>
        <Button
          variant="ghost"
          onClick={() => {
            const start = startOfWeek(todayIn(tz))
            setFrom(start)
            setTo(addDays(start, 6))
          }}
        >
          {t('common.thisWeek')}
        </Button>
        {canEdit && (
          <Button className="ml-auto" onClick={() => setEditing('new')}>
            <Plus className="h-4 w-4" aria-hidden="true" />
            {t('sch.newShift')}
          </Button>
        )}
      </Toolbar>

      {!validRange ? (
        <p role="alert" className="text-sm text-red-700">
          {t('rep.rangeTooLong', { n: 62 })}
        </p>
      ) : isLoading ? (
        <TableSkeleton />
      ) : error ? (
        <ErrorState error={error} onRetry={() => refetch()} />
      ) : byDay.length === 0 ? (
        <EmptyState message={t('sch.noShifts')} />
      ) : (
        <div className="space-y-4">
          {byDay.map(([day, shifts]) => (
            <section key={day} className="rounded-lg border border-gray-200 bg-white" aria-labelledby={`day-${day}`}>
              <h3 id={`day-${day}`} className="border-b border-gray-100 bg-gray-50 px-4 py-2 text-sm font-semibold text-gray-900">
                {formatDay(day, lang)}
              </h3>
              <ul className="divide-y divide-gray-100">
                {shifts.map((s) => (
                  <li key={s.id} className="flex flex-wrap items-center gap-3 px-4 py-2 text-sm">
                    <span className="w-28 tabular-nums text-gray-900">
                      {formatTime(s.start_time, tz)}–{formatTime(s.end_time, tz)}
                      {localDateOf(s.end_time, tz) !== day && <sup className="ml-0.5 text-xs text-gray-600">+1</sup>}
                    </span>
                    <span className="font-medium text-gray-900">{guardName(s.guard_id)}</span>
                    {s.template_id ? (
                      <Badge tone="blue">
                        <Repeat className="h-3 w-3" aria-hidden="true" />
                        {t('sch.fromTemplate')}
                      </Badge>
                    ) : (
                      <Badge>{t('sch.oneOff')}</Badge>
                    )}
                    {canEdit && (
                      <span className="ml-auto flex gap-1">
                        <Button size="sm" variant="ghost" onClick={() => setEditing(s)} aria-label={`${t('common.edit')} ${guardName(s.guard_id)} ${formatTime(s.start_time, tz)}`}>
                          <Pencil className="h-3.5 w-3.5" aria-hidden="true" />
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          className="text-red-700"
                          aria-label={`${t('common.delete')} ${guardName(s.guard_id)} ${formatTime(s.start_time, tz)}`}
                          loading={del.isPending && del.variables === s.id}
                          onClick={() => window.confirm(t('sch.confirmDeleteShift')) && del.mutate(s.id)}
                        >
                          <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
                        </Button>
                      </span>
                    )}
                  </li>
                ))}
              </ul>
            </section>
          ))}
        </div>
      )}

      {editing && (
        <ShiftModal site={site} shift={editing === 'new' ? null : editing} defaultDate={from} onClose={() => setEditing(null)} />
      )}
    </>
  )
}
