import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apiGet } from '../../api/client'
import { Field, Input } from '../../components/ui/Form'
import { Table, type Column } from '../../components/ui/Table'
import { useI18n } from '../../i18n/context'
import { formatNumber, monthIn } from '../../lib/time'
import { ReportShell } from './ReportShell'
import type { MonthlyRecapRow, RowsReport, Site } from '../../types'

export function MonthlyReport({ site }: { site: Site }) {
  const { t, lang } = useI18n()
  const [month, setMonth] = useState(() => monthIn(site.timezone))
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['report-monthly', site.id, month],
    queryFn: () => apiGet<RowsReport<MonthlyRecapRow>>('/reports/attendance/monthly', { site_id: site.id, month }),
    enabled: /^\d{4}-\d{2}$/.test(month),
  })

  const columns: Column<MonthlyRecapRow>[] = [
    { key: 'guard', header: t('common.guard'), cell: (r) => <span className="font-medium">{r.guard_name}</span> },
    { key: 'emp', header: t('att.employeeId'), cell: (r) => r.employee_id ?? '—' },
    { key: 'shifts', header: t('rep.scheduled'), numeric: true, cell: (r) => r.scheduled_shifts },
    { key: 'present', header: t('status.present'), numeric: true, cell: (r) => r.present },
    { key: 'late', header: t('status.late'), numeric: true, cell: (r) => r.late },
    { key: 'absent', header: t('status.absent'), numeric: true, cell: (r) => r.absent },
    { key: 'early', header: t('rep.earlyOut'), numeric: true, cell: (r) => r.early_out },
    { key: 'pending', header: t('status.pending'), numeric: true, cell: (r) => r.pending },
    { key: 'hours', header: t('rep.totalHours'), numeric: true, cell: (r) => formatNumber(r.total_hours, lang, 1) },
    { key: 'patrols', header: t('rep.patrols'), numeric: true, cell: (r) => r.patrol_sessions },
    { key: 'patrolPct', header: t('rep.patrolPct'), numeric: true, cell: (r) => formatNumber(r.patrol_completion_pct, lang, 1) },
  ]

  return (
    <ReportShell
      site={site}
      title={t('rep.monthly')}
      period={month}
      printable
      csv={{ path: '/reports/attendance/monthly', params: { site_id: site.id, month }, filename: `monthly-recap-${month}.csv` }}
      filters={
        <Field label={t('common.month')}>
          <Input type="month" value={month} onChange={(e) => setMonth(e.target.value)} />
        </Field>
      }
    >
      <Table columns={columns} rows={data?.rows} rowKey={(r) => r.guard_id} isLoading={isLoading} error={error} onRetry={() => refetch()} caption={t('rep.monthly')} />
    </ReportShell>
  )
}
