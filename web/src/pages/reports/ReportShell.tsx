import type { ReactNode } from 'react'
import { Printer } from 'lucide-react'
import { Button } from '../../components/ui/Button'
import { Notice, Toolbar } from '../../components/ui/States'
import { ExportButton } from '../../components/DownloadButton'
import type { ExportFormat } from '../../components/exportFormat'
import { useI18n } from '../../i18n/context'
import { formatDateTime } from '../../lib/time'
import type { Site } from '../../types'

interface Props {
  site: Site
  title: string
  period: string
  filters: ReactNode
  /** Download of the report (D-045): Excel + CSV, the incident log also PDF. */
  csv: { path: string; params: Record<string, string | number | undefined>; filename: string; disabled?: boolean; formats?: ExportFormat[] }
  /** Summary tiles above the table (a `StatRow`). */
  stats?: ReactNode
  /** Shown instead of stats + table when the filters are invalid. */
  invalid?: string
  children: ReactNode
}

/**
 * Report frame: filter toolbar with CSV / print actions, a print-only header
 * (site, period, generation time), summary tiles and the table.
 */
export function ReportShell({ site, title, period, filters, csv, stats, invalid, children }: Props) {
  const { t, lang } = useI18n()
  return (
    <div>
      <Toolbar>
        {filters}
        <div className="flex w-full flex-wrap gap-2 sm:ml-auto sm:w-auto">
          <ExportButton path={csv.path} params={csv.params} filename={csv.filename} formats={csv.formats} disabled={csv.disabled || !!invalid} />
          <Button variant="secondary" onClick={() => window.print()} disabled={!!invalid}>
            <Printer className="h-4 w-4" aria-hidden="true" />
            {t('p2.exp.print')}
          </Button>
        </div>
      </Toolbar>

      <header className="print-only mb-4 border-b border-gray-300 pb-3">
        <p className="text-xs font-semibold uppercase tracking-wider text-gray-500">{t('app.name')}</p>
        <h1 className="text-xl font-bold text-gray-900">{title}</h1>
        <p className="text-sm text-gray-700">
          {site.name}
          {site.address ? ` — ${site.address}` : ''} · {period}
        </p>
        <p className="mt-0.5 text-xs text-gray-500">{t('rep.generatedAt', { time: formatDateTime(new Date().toISOString(), site.timezone, lang) })}</p>
      </header>

      {invalid ? (
        <Notice tone="warn">{invalid}</Notice>
      ) : (
        <>
          {stats}
          {children}
        </>
      )}
    </div>
  )
}
