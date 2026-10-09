import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ChevronRight, History } from 'lucide-react'
import { apiGet } from '../../api/client'
import { TableSkeleton } from '../../components/LoadingSkeleton'
import { StatusBadge } from '../../components/ui/Badge'
import { EmptyState, ErrorState, Notice, Toolbar } from '../../components/ui/States'
import { useI18n } from '../../i18n/context'
import { daysBetween, formatDay, formatTime, localDateOf } from '../../lib/time'
import { fmtDuration, presetRange } from './format'
import { DateRangeFilter, MiniBar, Segmented } from './kit'
import { SessionDrawer } from './SessionDrawer'
import type { SessionStatus, SessionSummary, Site } from '../../types'

const MAX_DAYS = 62
type StatusFilter = 'all' | SessionStatus

export function SessionsTab({ site }: { site: Site }) {
  const { t, lang } = useI18n()
  const tz = site.timezone
  const [range, setRange] = useState(() => presetRange('7d', tz))
  const [status, setStatus] = useState<StatusFilter>('all')
  const [selected, setSelected] = useState<SessionSummary | null>(null)
  const { from, to } = range
  const valid = !!from && !!to && from <= to && daysBetween(from, to) <= MAX_DAYS

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['patrol-sessions', site.id, from, to],
    queryFn: () => apiGet<SessionSummary[]>('/patrol/sessions', { site_id: site.id, from, to }),
    enabled: valid,
  })

  const all = useMemo(() => data ?? [], [data])
  const rows = status === 'all' ? all : all.filter((r) => r.status === status)
  const count = (s: SessionStatus) => all.filter((r) => r.status === s).length
  const avg = all.length ? Math.round(all.reduce((n, r) => n + r.completion_pct, 0) / all.length) : 0
  const breaches = all.reduce((n, r) => n + r.sla_breaches, 0)

  return (
    <>
      <Toolbar>
        <DateRangeFilter from={from} to={to} tz={tz} onChange={setRange} />
        <Segmented<StatusFilter>
          label={t('common.status')}
          value={status}
          onChange={setStatus}
          options={[
            { value: 'all', label: t('common.all'), count: all.length },
            { value: 'in_progress', label: t('status.in_progress'), count: count('in_progress'), tone: 'blue' },
            { value: 'completed', label: t('status.completed'), count: count('completed'), tone: 'green' },
            { value: 'abandoned', label: t('status.abandoned'), count: count('abandoned'), tone: 'purple' },
          ]}
        />
      </Toolbar>

      {!valid ? (
        <Notice tone="warn">{t('ops.range.invalid', { n: MAX_DAYS })}</Notice>
      ) : isLoading ? (
        <TableSkeleton cols={6} />
      ) : error ? (
        <ErrorState error={error} onRetry={() => refetch()} />
      ) : all.length === 0 ? (
        <EmptyState icon={History} title={t('ops.pat.histEmptyTitle')} message={t('ops.pat.histEmptyBody')} />
      ) : (
        <>
          <p className="mb-3 text-sm text-gray-600">{t('ops.pat.histSummary', { n: all.length, c: avg, b: breaches })}</p>
          {rows.length === 0 ? (
            <EmptyState message={t('ops.filterEmpty')} />
          ) : (
            <>
              {/* Phones: compact list instead of a wide table. */}
              <ul className="divide-y divide-gray-100 overflow-hidden rounded-2xl border border-gray-200/80 bg-white shadow-card sm:hidden">
                {rows.map((r) => (
                  <li key={r.session_id}>
                    <button
                      type="button"
                      onClick={() => setSelected(r)}
                      className="w-full px-4 py-3 text-left focus:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500"
                    >
                      <div className="flex items-start justify-between gap-2">
                        <p className="min-w-0 truncate text-sm font-semibold text-gray-900">{r.route_name}</p>
                        <StatusBadge status={r.status} />
                      </div>
                      <p className="mt-0.5 text-xs text-gray-500">
                        {r.guard_name} · {formatDay(localDateOf(r.started_at, tz), lang, false)} {formatTime(r.started_at, tz)}
                      </p>
                      <div className="mt-2 flex items-center gap-2">
                        <MiniBar pct={r.completion_pct} label={`${r.route_name} — ${r.guard_name}`} />
                        <span className="shrink-0 text-xs tabular-nums text-gray-600">
                          {r.scanned_count}/{r.total_checkpoints}
                        </span>
                        {r.sla_breaches > 0 && (
                          <span className="shrink-0 rounded-full bg-orange-50 px-2 py-0.5 text-[11px] font-semibold text-orange-700 ring-1 ring-inset ring-orange-200">
                            {t('ops.pat.breachShort', { n: r.sla_breaches })}
                          </span>
                        )}
                      </div>
                    </button>
                  </li>
                ))}
              </ul>
              <div className="scroll-thin relative hidden overflow-x-auto rounded-2xl border border-gray-200/80 bg-white shadow-card sm:block">
                <table className="min-w-full text-sm">
                  <caption className="sr-only">{t('ops.pat.tabHistory')}</caption>
                  <thead className="border-b border-gray-200 bg-gray-50/80">
                    <tr className="text-left text-[11px] font-semibold uppercase tracking-wider text-gray-500">
                      <th scope="col" className="whitespace-nowrap px-4 py-2.5">{t('pat.route')}</th>
                      <th scope="col" className="whitespace-nowrap px-4 py-2.5">{t('common.guard')}</th>
                      <th scope="col" className="whitespace-nowrap px-4 py-2.5">{t('ops.pat.time')}</th>
                      <th scope="col" className="whitespace-nowrap px-4 py-2.5">{t('common.status')}</th>
                      <th scope="col" className="whitespace-nowrap px-4 py-2.5">{t('pat.completion')}</th>
                      <th scope="col" className="whitespace-nowrap px-4 py-2.5 text-right">{t('pat.slaBreaches')}</th>
                      <th scope="col" className="px-2 py-2.5">
                        <span className="sr-only">{t('common.actions')}</span>
                      </th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-100">
                    {rows.map((r) => (
                      <tr key={r.session_id} onClick={() => setSelected(r)} className="cursor-pointer transition-colors hover:bg-gray-50/80">
                        <td className="px-4 py-3">
                          <button
                            type="button"
                            onClick={(e) => {
                              e.stopPropagation()
                              setSelected(r)
                            }}
                            className="text-left font-medium text-gray-900 hover:text-primary-700 focus:outline-none focus-visible:rounded focus-visible:ring-2 focus-visible:ring-primary-500"
                          >
                            {r.route_name}
                          </button>
                        </td>
                        <td className="whitespace-nowrap px-4 py-3 text-gray-700">{r.guard_name}</td>
                        <td className="whitespace-nowrap px-4 py-3">
                          <div className="text-gray-800">
                            {formatTime(r.started_at, tz)} – {r.ended_at ? formatTime(r.ended_at, tz) : '…'}
                            {r.ended_at && <span className="ml-1.5 text-xs text-gray-400">{fmtDuration(Date.parse(r.ended_at) - Date.parse(r.started_at), t)}</span>}
                          </div>
                          <div className="text-xs text-gray-500">{formatDay(localDateOf(r.started_at, tz), lang)}</div>
                        </td>
                        <td className="px-4 py-3">
                          <StatusBadge status={r.status} />
                        </td>
                        <td className="px-4 py-3">
                          <div className="flex w-44 items-center gap-2">
                            <MiniBar pct={r.completion_pct} label={`${r.route_name} — ${r.guard_name}`} />
                            <span className="shrink-0 whitespace-nowrap text-xs tabular-nums text-gray-600">
                              {Math.round(r.completion_pct)}% <span className="text-gray-400">· {r.scanned_count}/{r.total_checkpoints}</span>
                            </span>
                          </div>
                        </td>
                        <td className="px-4 py-3 text-right tabular-nums">
                          {r.sla_breaches ? (
                            <span className="inline-flex min-w-[1.75rem] justify-center rounded-full bg-orange-50 px-2 py-0.5 text-xs font-semibold text-orange-700 ring-1 ring-inset ring-orange-200">
                              {r.sla_breaches}
                            </span>
                          ) : (
                            <span className="text-gray-400">0</span>
                          )}
                        </td>
                        <td className="px-2 py-3 text-gray-300">
                          <ChevronRight className="h-4 w-4" aria-hidden="true" />
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}
        </>
      )}
      {selected && <SessionDrawer site={site} sessionId={selected.session_id} guardName={selected.guard_name} onClose={() => setSelected(null)} />}
    </>
  )
}
