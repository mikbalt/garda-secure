import type { ReactNode } from 'react'
import { Printer } from 'lucide-react'
import { Button } from '../../components/ui/Button'
import { DownloadCsvButton } from '../../components/DownloadButton'
import { useI18n } from '../../i18n/context'
import { formatDateTime } from '../../lib/time'
import type { Site } from '../../types'

interface Props {
  site: Site
  title: string
  period: string
  filters: ReactNode
  csv: { path: string; params: Record<string, string | number | undefined>; filename: string; disabled?: boolean }
  printable?: boolean
  children: ReactNode
}

/** Filters + CSV download (+ optional print) around a report table. */
export function ReportShell({ site, title, period, filters, csv, printable, children }: Props) {
  const { t, lang } = useI18n()
  return (
    <div>
      <div className="mb-4 flex flex-wrap items-end gap-3 no-print">
        {filters}
        <div className="ml-auto flex gap-2">
          <DownloadCsvButton path={csv.path} params={csv.params} filename={csv.filename} label={t('common.downloadCsv')} disabled={csv.disabled} />
          {printable && (
            <Button variant="secondary" onClick={() => window.print()}>
              <Printer className="h-4 w-4" aria-hidden="true" />
              {t('common.print')}
            </Button>
          )}
        </div>
      </div>
      <header className="print-only mb-4">
        <h1 className="text-lg font-bold">
          {t('app.name')} — {title}
        </h1>
        <p className="text-sm">
          {site.name} · {period}
        </p>
        <p className="text-xs text-gray-600">{t('rep.generatedAt', { time: formatDateTime(new Date().toISOString(), site.timezone, lang) })}</p>
      </header>
      {children}
    </div>
  )
}
