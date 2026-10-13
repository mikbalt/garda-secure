import { useState } from 'react'
import { ChevronDown, Download, FileSpreadsheet, FileText, Sheet, type LucideIcon } from 'lucide-react'
import { downloadFile } from '../api/client'
import { useT, type Key } from '../i18n/context'
import { useDownloadErrorToast } from './toast/context'
import { Button } from './ui/Button'
import { Popover } from './ui/Popover'
import { exportFilename, type ExportFormat } from './exportFormat'

const META: Record<ExportFormat, { icon: LucideIcon; label: Key; hint: Key }> = {
  xlsx: { icon: FileSpreadsheet, label: 'p2.exp.xlsx', hint: 'p2.exp.xlsxHint' },
  csv: { icon: Sheet, label: 'p2.exp.csv', hint: 'p2.exp.csvHint' },
  pdf: { icon: FileText, label: 'p2.exp.pdf', hint: 'p2.exp.pdfHint' },
}

interface ExportProps {
  path: string
  params: Record<string, string | number | undefined>
  /** File name without (or with any) extension; the server's name wins. */
  filename: string
  formats?: ExportFormat[]
  label?: string
  disabled?: boolean
}

/**
 * "Download ▾" disclosure offering the report formats (Excel, CSV, PDF).
 * Errors (`export_too_large`, `invalid_format`, …) are shown as a toast.
 */
export function ExportButton({ path, params, filename, formats = ['xlsx', 'csv'], label, disabled }: ExportProps) {
  const t = useT()
  const onError = useDownloadErrorToast()
  const [busy, setBusy] = useState<ExportFormat | null>(null)
  const run = async (format: ExportFormat) => {
    setBusy(format)
    try {
      await downloadFile(path, { ...params, format }, exportFilename(filename, format))
    } catch (err) {
      await onError(err)
    } finally {
      setBusy(null)
    }
  }
  return (
    <Popover
      align="right"
      label={t('p2.exp.menu')}
      panelClassName="w-72 max-w-[calc(100vw-2rem)]"
      trigger={({ toggle, open: _open, ...aria }) => (
        <Button variant="secondary" onClick={toggle} {...aria} loading={!!busy} disabled={disabled}>
          {!busy && <Download className="h-4 w-4" aria-hidden="true" />}
          {label ?? t('p2.exp.download')}
          <ChevronDown className="h-3.5 w-3.5 text-gray-500" aria-hidden="true" />
        </Button>
      )}
    >
      {(close) => (
        <ul>
          {formats.map((f) => {
            const m = META[f]
            const Icon = m.icon
            return (
              <li key={f}>
                <button
                  type="button"
                  onClick={() => {
                    close()
                    void run(f)
                  }}
                  className="flex w-full items-start gap-3 px-3.5 py-2.5 text-left hover:bg-gray-50 focus:outline-none focus-visible:bg-gray-50 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500"
                >
                  <Icon className="mt-0.5 h-4 w-4 shrink-0 text-gray-500" aria-hidden="true" />
                  <span className="min-w-0">
                    <span className="block text-sm font-medium text-gray-900">{t(m.label)}</span>
                    <span className="block text-xs text-gray-500">{t(m.hint)}</span>
                  </span>
                </button>
              </li>
            )
          })}
        </ul>
      )}
    </Popover>
  )
}

interface Props {
  path: string
  params: Record<string, string | number | undefined>
  filename: string
  label?: string
  disabled?: boolean
}

/** Report download (kept for existing callers): Excel or CSV. */
export function DownloadCsvButton({ path, params, filename, label, disabled }: Props) {
  return <ExportButton path={path} params={params} filename={filename} label={label} disabled={disabled} />
}
