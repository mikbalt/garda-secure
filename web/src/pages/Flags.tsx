import { useId, useState, type FormEvent } from 'react'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, ExternalLink, Filter, Flag, RotateCw } from 'lucide-react'
import { Link } from 'react-router-dom'
import apiClient, { apiGet } from '../api/client'
import { FlagBadges } from '../components/FlagBadges'
import { useErrorToast, useToast } from '../components/toast/context'
import { Button } from '../components/ui/Button'
import { Field, Input, Select } from '../components/ui/Form'
import { EmptyState, PageHeader, Toolbar } from '../components/ui/States'
import { Table, type Column } from '../components/ui/Table'
import { useSite } from '../context/site'
import { useGuards } from '../hooks/useGuards'
import { translateMaybe, useI18n } from '../i18n/context'
import { flagRecordLink, flagText, KNOWN_FLAGS } from '../lib/flags'
import { addDays, daysBetween, formatDateTime, todayIn, zonedToUtcIso } from '../lib/time'
import type { FlagPage, FlagRow } from '../types'

const PAGE_SIZE = 50
const DEFAULT_TZ = 'Asia/Jakarta'
/** The API accepts at most this many days per request. */
const MAX_DAYS = 92

interface Filters {
  site: string
  guard: string
  flag: string
  status: '' | 'false' | 'true'
  from: string
  to: string
}

/** Fraud-signal dashboard (TAQ-21): plausibility flags per guard and site. Read-only for the records. */
export default function Flags() {
  const { t, lang } = useI18n()
  const { sites, site, setSiteId } = useSite()
  const toast = useToast()
  const onError = useErrorToast()
  const qc = useQueryClient()
  // Days are read in the time zone of the site picked in the top bar.
  const tz = site?.timezone ?? DEFAULT_TZ
  const today = todayIn(tz)
  const initial: Filters = { site: '', guard: '', flag: '', status: 'false', from: addDays(today, -6), to: today }
  const [draft, setDraft] = useState<Filters>(initial)
  const [applied, setApplied] = useState<Filters>(initial)
  const [offset, setOffset] = useState(0)
  const formId = useId()
  const tooLong = !!draft.from && !!draft.to && daysBetween(draft.from, draft.to) > MAX_DAYS
  const valid = !!draft.from && !!draft.to && draft.from <= draft.to && !tooLong
  const siteTz = (id: string) => sites.find((s) => s.id === id)?.timezone ?? tz
  const { guards } = useGuards(draft.site)

  const { data, isLoading, isFetching, error, refetch } = useQuery({
    queryKey: ['flags', applied, offset, tz],
    queryFn: () =>
      apiGet<FlagPage>('/flags', {
        site_id: applied.site,
        guard_id: applied.guard,
        flag: applied.flag,
        reviewed: applied.status,
        from: zonedToUtcIso(applied.from, '00:00', tz),
        to: zonedToUtcIso(addDays(applied.to, 1), '00:00', tz),
        limit: PAGE_SIZE,
        offset,
      }),
    placeholderData: keepPreviousData,
  })

  const review = useMutation({
    mutationFn: (r: FlagRow) => apiClient.post<{ already_reviewed?: boolean; reviewed_by?: string }>('/flags/review', { kind: r.kind, record_id: r.record_id, flag: r.flag }),
    onSuccess: ({ data }) => {
      if (data.already_reviewed) toast.info(t('fd.alreadyReviewed', { name: data.reviewed_by ?? '—' }))
      else toast.success(t('fd.reviewedDone'))
      qc.invalidateQueries({ queryKey: ['flags'] })
    },
    onError,
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!valid) return
    setApplied(draft)
    setOffset(0)
  }

  const columns: Column<FlagRow>[] = [
    {
      key: 'when',
      header: t('fd.when'),
      cell: (r) => <span className="whitespace-nowrap tabular-nums text-gray-800">{formatDateTime(r.event_at, siteTz(r.site_id), lang)}</span>,
    },
    {
      key: 'guard',
      header: t('fd.guard'),
      cell: (r) => (
        <span className="block min-w-[8rem]">
          <span className="block font-medium text-gray-900">{r.guard_name}</span>
          <span className="block text-xs text-gray-600">{r.site_name}</span>
        </span>
      ),
    },
    {
      key: 'flag',
      header: t('fd.flag'),
      cell: (r) => (
        <span className="flex flex-wrap items-center gap-1.5">
          <FlagBadges flags={[r.flag]} />
          <span className="text-xs text-gray-600">{translateMaybe(lang, `fd.kind.${r.kind}`, r.kind)}</span>
        </span>
      ),
    },
    {
      key: 'record',
      header: t('fd.record'),
      cell: (r) => (
        <Link
          to={flagRecordLink(r, siteTz(r.site_id))}
          onClick={() => setSiteId(r.site_id)}
          aria-label={t('fd.openRecordFor', { flag: flagText(r.flag, lang).label, guard: r.guard_name })}
          className="inline-flex items-center gap-1 text-sm font-medium text-primary-700 hover:underline focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
        >
          {t('fd.openRecord')}
          <ExternalLink className="h-3.5 w-3.5" aria-hidden="true" />
        </Link>
      ),
    },
    {
      key: 'review',
      header: t('fd.review'),
      stickyEnd: true,
      cell: (r) =>
        r.reviewed_at ? (
          <span className="text-xs text-gray-700">
            {t('fd.reviewedBy', { name: r.reviewed_by ?? '—' })}
            <span className="block text-gray-600">{formatDateTime(r.reviewed_at, siteTz(r.site_id), lang)}</span>
          </span>
        ) : (
          <Button
            variant="secondary"
            size="sm"
            loading={review.isPending && review.variables?.record_id === r.record_id && review.variables.flag === r.flag}
            onClick={() => review.mutate(r)}
            aria-label={t('fd.markReviewedFor', { flag: flagText(r.flag, lang).label, guard: r.guard_name })}
          >
            {t('fd.markReviewed')}
          </Button>
        ),
    },
  ]

  const rows = data?.rows ?? []
  const page = Math.floor(offset / PAGE_SIZE) + 1

  return (
    <div>
      <PageHeader icon={Flag} title={t('fd.title')} subtitle={t('fd.subtitle')} />
      <Toolbar>
        <form id={formId} onSubmit={submit} className="flex w-full flex-wrap items-end gap-3" noValidate>
          <Field label={t('fd.site')} className="w-44">
            <Select value={draft.site} onChange={(e) => setDraft((d) => ({ ...d, site: e.target.value, guard: '' }))}>
              <option value="">{t('common.all')}</option>
              {sites.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
            </Select>
          </Field>
          <Field label={t('fd.guard')} className="w-44">
            <Select value={draft.guard} disabled={!draft.site} onChange={(e) => setDraft((d) => ({ ...d, guard: e.target.value }))}>
              <option value="">{t('common.all')}</option>
              {guards.map((g) => (
                <option key={g.id} value={g.id}>
                  {g.name}
                </option>
              ))}
            </Select>
          </Field>
          <Field label={t('fd.flag')} className="w-48">
            <Select value={draft.flag} onChange={(e) => setDraft((d) => ({ ...d, flag: e.target.value }))}>
              <option value="">{t('common.all')}</option>
              {KNOWN_FLAGS.map((f) => (
                <option key={f} value={f}>
                  {flagText(f, lang).label}
                </option>
              ))}
            </Select>
          </Field>
          <Field label={t('fd.status')} className="w-40">
            <Select value={draft.status} onChange={(e) => setDraft((d) => ({ ...d, status: e.target.value as Filters['status'] }))}>
              <option value="">{t('fd.statusAll')}</option>
              <option value="false">{t('fd.statusOpen')}</option>
              <option value="true">{t('fd.statusReviewed')}</option>
            </Select>
          </Field>
          <Field label={t('common.from')} className="w-40" error={!valid ? t(tooLong ? 'fd.rangeTooLong' : 'fd.rangeInvalid') : undefined}>
            <Input type="date" value={draft.from} max={today} onChange={(e) => setDraft((d) => ({ ...d, from: e.target.value }))} />
          </Field>
          <Field label={t('common.to')} className="w-40">
            <Input type="date" value={draft.to} max={today} onChange={(e) => setDraft((d) => ({ ...d, to: e.target.value }))} />
          </Field>
          <div className="flex gap-2">
            <Button type="submit" disabled={!valid}>
              <Filter className="h-4 w-4" aria-hidden="true" />
              {t('fd.apply')}
            </Button>
            <Button variant="secondary" onClick={() => refetch()} loading={isFetching && !isLoading} aria-label={t('common.refresh')}>
              {!(isFetching && !isLoading) && <RotateCw className="h-4 w-4" aria-hidden="true" />}
            </Button>
          </div>
        </form>
      </Toolbar>
      <p className="mb-3 text-xs text-gray-600">{t('fd.tzNote', { tz })}</p>

      {!isLoading && !error && rows.length === 0 ? (
        <EmptyState icon={Flag} title={t('fd.emptyTitle')} message={t('fd.empty')} />
      ) : (
        <Table columns={columns} rows={rows} rowKey={(r) => `${r.kind}:${r.record_id}:${r.flag}`} isLoading={isLoading} error={error} onRetry={() => refetch()} caption={t('fd.title')} />
      )}

      {(offset > 0 || data?.has_more) && (
        <nav aria-label={t('fd.pages')} className="mt-4 flex items-center justify-between gap-3">
          <Button variant="secondary" disabled={offset === 0 || isFetching} onClick={() => setOffset((o) => Math.max(0, o - PAGE_SIZE))}>
            <ChevronLeft className="h-4 w-4" aria-hidden="true" />
            {t('fd.prev')}
          </Button>
          <span className="text-sm text-gray-700" aria-live="polite">
            {t('fd.page', { n: page })}
          </span>
          <Button variant="secondary" disabled={!data?.has_more || isFetching} onClick={() => setOffset((o) => o + PAGE_SIZE)}>
            {t('fd.next')}
            <ChevronRight className="h-4 w-4" aria-hidden="true" />
          </Button>
        </nav>
      )}
    </div>
  )
}
