import { useId, useState, type FormEvent } from 'react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, FileClock, Filter, RotateCw } from 'lucide-react'
import { apiGet } from '../api/client'
import { Badge, type Tone } from '../components/ui/Badge'
import { Button } from '../components/ui/Button'
import { Field, Input, Select } from '../components/ui/Form'
import { Modal } from '../components/ui/Modal'
import { EmptyState, PageHeader, Toolbar } from '../components/ui/States'
import { Table, type Column } from '../components/ui/Table'
import { useSite } from '../context/site'
import { useI18n, translateMaybe } from '../i18n/context'
import { diffJson, showValue } from '../lib/diff'
import { addDays, formatDateTime, todayIn, zonedToUtcIso } from '../lib/time'
import type { AuditPage, AuditRow } from '../types'

const PAGE_SIZE = 50
/** `resource_type` filter values (first path segment matches all below it). */
const RESOURCE_TYPES = [
  'sites',
  'guards',
  'devices',
  'shift-templates',
  'shifts',
  'attendance',
  'patrol',
  'incidents',
  'sos',
  'defects',
  'visitors',
  'media',
  'notifications',
  'settings',
  'backups',
]

const methodTone: Record<string, Tone> = { POST: 'green', PUT: 'blue', PATCH: 'blue', DELETE: 'red' }

function statusTone(code: number): Tone {
  if (code >= 500) return 'red'
  if (code >= 400) return 'orange'
  return 'green'
}

interface Filters {
  from: string
  to: string
  actor: string
  resource: string
}

function Json({ value, label }: { value: unknown; label: string }) {
  return (
    <pre aria-label={label} tabIndex={0} className="scroll-thin max-h-72 overflow-auto rounded-xl bg-gray-900 p-3.5 text-xs leading-relaxed text-gray-100">
      {JSON.stringify(value, null, 2)}
    </pre>
  )
}

function DiffTable({ before, after }: { before: unknown; after: unknown }) {
  const { t } = useI18n()
  const [showAll, setShowAll] = useState(false)
  const rows = diffJson(before, after)
  const changed = rows.filter((r) => r.changed)
  const shown = showAll ? rows : changed
  return (
    <div>
      <div tabIndex={0} role="region" aria-label={t('aud.diff')} className="scroll-thin overflow-x-auto rounded-xl border border-gray-200 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500">
        <table className="min-w-full text-sm">
          <caption className="sr-only">{t('aud.diff')}</caption>
          <thead className="bg-gray-50 text-left text-[11px] font-semibold uppercase tracking-wider text-gray-600">
            <tr>
              <th scope="col" className="px-3 py-2">
                {t('aud.field')}
              </th>
              <th scope="col" className="px-3 py-2">
                {t('aud.before')}
              </th>
              <th scope="col" className="px-3 py-2">
                {t('aud.after')}
              </th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-100">
            {shown.length === 0 ? (
              <tr>
                <td colSpan={3} className="px-3 py-3 text-gray-600">
                  {t('aud.noChanges')}
                </td>
              </tr>
            ) : (
              shown.map((r) => (
                <tr key={r.path} className={r.changed ? 'bg-amber-50/60' : ''}>
                  <th scope="row" className="whitespace-nowrap px-3 py-2 text-left font-mono text-xs font-medium text-gray-800">
                    {r.path}
                    {r.changed && <span className="sr-only"> ({t('aud.changed')})</span>}
                  </th>
                  <td className="break-all px-3 py-2 font-mono text-xs text-red-800">{showValue(r.before)}</td>
                  <td className="break-all px-3 py-2 font-mono text-xs text-emerald-800">{showValue(r.after)}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
      {rows.length > changed.length && (
        <Button variant="ghost" size="sm" className="mt-2" onClick={() => setShowAll((v) => !v)} aria-pressed={showAll}>
          {showAll ? t('aud.onlyChanged') : t('aud.showUnchanged', { n: rows.length - changed.length })}
        </Button>
      )}
    </div>
  )
}

function AuditDrawer({ row, tz, onClose }: { row: AuditRow; tz: string; onClose: () => void }) {
  const { t, lang } = useI18n()
  const hasDiff = row.before !== undefined || row.after !== undefined
  const meta: [string, string | undefined][] = [
    [t('aud.when'), formatDateTime(row.created_at, tz, lang)],
    [t('aud.actor'), row.actor ?? row.user_id],
    [t('aud.roles'), row.roles?.join(', ')],
    [t('aud.guard'), row.guard_id],
    [t('aud.resource'), [row.resource_type, row.resource_id].filter(Boolean).join(' · ') || undefined],
    [t('aud.status'), String(row.status_code)],
    [t('aud.requestId'), row.request_id],
    [t('aud.ip'), row.ip],
    [t('aud.userAgent'), row.user_agent],
  ]
  return (
    <Modal open onClose={onClose} variant="drawer" title={t('aud.detail')}>
      <div className="space-y-6">
        <div>
          <p className="flex flex-wrap items-center gap-2 font-mono text-sm text-gray-900">
            {row.method && <Badge tone={methodTone[row.method] ?? 'gray'}>{row.method}</Badge>}
            <span className="break-all">{row.route ?? row.action}</span>
          </p>
        </div>
        <dl className="grid grid-cols-1 gap-x-4 gap-y-2.5 text-sm sm:grid-cols-[10rem_minmax(0,1fr)]">
          {meta
            .filter(([, v]) => v)
            .map(([k, v]) => (
              <div key={k} className="contents">
                <dt className="font-medium text-gray-600">{k}</dt>
                <dd className="break-all text-gray-900">{v}</dd>
              </div>
            ))}
        </dl>
        {row.params && Object.keys(row.params).length > 0 && (
          <section>
            <h3 className="mb-2 text-sm font-semibold text-gray-900">{t('aud.params')}</h3>
            <Json value={row.params} label={t('aud.params')} />
          </section>
        )}
        {hasDiff && (
          <section>
            <h3 className="mb-1 text-sm font-semibold text-gray-900">{t('aud.diff')}</h3>
            <p className="mb-2 text-xs text-gray-600">{t('aud.diffHint')}</p>
            <DiffTable before={row.before} after={row.after} />
          </section>
        )}
        {row.summary !== undefined && row.summary !== null && (
          <section>
            <h3 className="mb-1 text-sm font-semibold text-gray-900">{t('aud.summary')}</h3>
            <p className="mb-2 text-xs text-gray-600">{t('aud.summaryHint')}</p>
            <Json value={row.summary} label={t('aud.summary')} />
          </section>
        )}
      </div>
    </Modal>
  )
}

/** Audit log of the realm (admin, NFR-05 / F12): who changed what, with before/after. */
export default function Audit() {
  const { t, lang } = useI18n()
  const { site } = useSite()
  const tz = site?.timezone ?? 'Asia/Jakarta'
  const today = todayIn(tz)
  const initial: Filters = { from: addDays(today, -6), to: today, actor: '', resource: '' }
  const [draft, setDraft] = useState<Filters>(initial)
  const [applied, setApplied] = useState<Filters>(initial)
  const [offset, setOffset] = useState(0)
  const [open, setOpen] = useState<AuditRow | null>(null)
  const formId = useId()
  const valid = !!draft.from && !!draft.to && draft.from <= draft.to

  const { data, isLoading, isFetching, error, refetch } = useQuery({
    queryKey: ['audit', applied, offset, tz],
    queryFn: () =>
      apiGet<AuditPage>('/audit', {
        from: zonedToUtcIso(applied.from, '00:00', tz),
        to: zonedToUtcIso(addDays(applied.to, 1), '00:00', tz),
        actor: applied.actor.trim(),
        resource_type: applied.resource,
        limit: PAGE_SIZE,
        offset,
      }),
    placeholderData: keepPreviousData,
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!valid) return
    setApplied(draft)
    setOffset(0)
  }

  const columns: Column<AuditRow>[] = [
    {
      key: 'when',
      header: t('aud.when'),
      cell: (r) => <span className="whitespace-nowrap tabular-nums text-gray-800">{formatDateTime(r.created_at, tz, lang)}</span>,
    },
    {
      key: 'actor',
      header: t('aud.actor'),
      cell: (r) => (
        <span className="block min-w-[8rem]">
          <span className="block font-medium text-gray-900">{r.actor ?? r.user_id}</span>
          {r.roles?.length ? <span className="block text-xs text-gray-600">{r.roles.join(', ')}</span> : null}
        </span>
      ),
    },
    {
      key: 'action',
      header: t('aud.action'),
      cell: (r) => (
        <button
          type="button"
          onClick={() => setOpen(r)}
          className="flex min-w-[14rem] items-center gap-2 text-left focus:outline-none focus-visible:rounded focus-visible:ring-2 focus-visible:ring-primary-500"
          aria-label={`${t('aud.openDetail')} — ${r.action}`}
        >
          {r.method && <Badge tone={methodTone[r.method] ?? 'gray'}>{r.method}</Badge>}
          <span className="break-all font-mono text-xs text-gray-900 underline decoration-gray-300 underline-offset-2">{r.route ?? r.action}</span>
        </button>
      ),
    },
    {
      key: 'resource',
      header: t('aud.resource'),
      cell: (r) => (
        <span className="block min-w-[8rem]">
          <span className="block text-gray-800">{r.resource_type ? translateMaybe(lang, `aud.res.${r.resource_type.split('.')[0]}`, r.resource_type) : '—'}</span>
          {r.resource_id && <span className="block truncate font-mono text-xs text-gray-600">{r.resource_id.slice(0, 8)}…</span>}
        </span>
      ),
    },
    {
      key: 'status',
      header: t('aud.status'),
      cell: (r) => (
        <span className="flex flex-wrap gap-1">
          <Badge tone={statusTone(r.status_code)}>{r.status_code}</Badge>
          {(r.before !== undefined || r.after !== undefined) && <Badge tone="purple">{t('aud.hasDiff')}</Badge>}
        </span>
      ),
    },
  ]

  const rows = data?.rows ?? []
  const page = Math.floor(offset / PAGE_SIZE) + 1

  return (
    <div>
      <PageHeader icon={FileClock} title={t('aud.title')} subtitle={t('aud.subtitle')} />
      <Toolbar>
        <form id={formId} onSubmit={submit} className="flex w-full flex-wrap items-end gap-3" noValidate>
          <Field label={t('common.from')} className="w-40" error={!valid ? t('aud.rangeInvalid') : undefined}>
            <Input type="date" value={draft.from} max={today} onChange={(e) => setDraft((d) => ({ ...d, from: e.target.value }))} />
          </Field>
          <Field label={t('common.to')} className="w-40">
            <Input type="date" value={draft.to} max={today} onChange={(e) => setDraft((d) => ({ ...d, to: e.target.value }))} />
          </Field>
          <Field label={t('aud.actor')} className="w-48">
            <Input value={draft.actor} onChange={(e) => setDraft((d) => ({ ...d, actor: e.target.value }))} placeholder={t('aud.actorPh')} autoComplete="off" />
          </Field>
          <Field label={t('aud.resource')} className="w-48">
            <Select value={draft.resource} onChange={(e) => setDraft((d) => ({ ...d, resource: e.target.value }))}>
              <option value="">{t('common.all')}</option>
              {RESOURCE_TYPES.map((r) => (
                <option key={r} value={r}>
                  {translateMaybe(lang, `aud.res.${r}`, r)}
                </option>
              ))}
            </Select>
          </Field>
          <div className="flex gap-2">
            <Button type="submit" disabled={!valid}>
              <Filter className="h-4 w-4" aria-hidden="true" />
              {t('aud.apply')}
            </Button>
            <Button variant="secondary" onClick={() => refetch()} loading={isFetching && !isLoading} aria-label={t('common.refresh')}>
              {!(isFetching && !isLoading) && <RotateCw className="h-4 w-4" aria-hidden="true" />}
            </Button>
          </div>
        </form>
      </Toolbar>
      <p className="mb-3 text-xs text-gray-600">{t('aud.tzNote', { tz })}</p>

      {!isLoading && !error && rows.length === 0 ? (
        <EmptyState icon={FileClock} title={t('aud.emptyTitle')} message={t('aud.empty')} />
      ) : (
        <Table columns={columns} rows={rows} rowKey={(r) => r.id} isLoading={isLoading} error={error} onRetry={() => refetch()} caption={t('aud.title')} />
      )}

      {(offset > 0 || data?.has_more) && (
        <nav aria-label={t('aud.pages')} className="mt-4 flex items-center justify-between gap-3">
          <Button variant="secondary" disabled={offset === 0 || isFetching} onClick={() => setOffset((o) => Math.max(0, o - PAGE_SIZE))}>
            <ChevronLeft className="h-4 w-4" aria-hidden="true" />
            {t('aud.prev')}
          </Button>
          <span className="text-sm text-gray-700" aria-live="polite">
            {t('aud.page', { n: page })}
          </span>
          <Button variant="secondary" disabled={!data?.has_more || isFetching} onClick={() => setOffset((o) => o + PAGE_SIZE)}>
            {t('aud.next')}
            <ChevronRight className="h-4 w-4" aria-hidden="true" />
          </Button>
        </nav>
      )}
      {open && <AuditDrawer row={open} tz={tz} onClose={() => setOpen(null)} />}
    </div>
  )
}
