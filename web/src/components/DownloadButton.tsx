import { useState } from 'react'
import { Download } from 'lucide-react'
import { downloadFile } from '../api/client'
import { useT } from '../i18n/context'
import { useDownloadErrorToast } from './toast/context'
import { Button } from './ui/Button'

interface Props {
  path: string
  params: Record<string, string | number | undefined>
  filename: string
  label?: string
  disabled?: boolean
}

/** Downloads a CSV report with the bearer token (`format=csv`). */
export function DownloadCsvButton({ path, params, filename, label, disabled }: Props) {
  const t = useT()
  const onError = useDownloadErrorToast()
  const [busy, setBusy] = useState(false)
  const run = async () => {
    setBusy(true)
    try {
      await downloadFile(path, { ...params, format: 'csv' }, filename)
    } catch (err) {
      await onError(err)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Button variant="secondary" onClick={run} loading={busy} disabled={disabled}>
      {!busy && <Download className="h-4 w-4" aria-hidden="true" />}
      {label ?? t('common.exportCsv')}
    </Button>
  )
}
