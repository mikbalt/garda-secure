import { useState } from 'react'
import { FileArchive, FileCheck2, FolderClock } from 'lucide-react'
import { downloadFile } from '../../api/client'
import { useDownloadErrorToast } from '../../components/toast/context'
import { Button } from '../../components/ui/Button'
import { Field, Select } from '../../components/ui/Form'
import { Notice, Toolbar } from '../../components/ui/States'
import { useI18n } from '../../i18n/context'
import { monthIn } from '../../lib/time'
import { addMonths } from './format'
import { MonthPicker } from './ReportKit'
import type { Site } from '../../types'

type Busy = 'pdf' | 'kept' | 'bundle' | null

/** The last day of "YYYY-MM". */
function monthEnd(month: string): string {
  return new Date(Date.UTC(Number(month.slice(0, 4)), Number(month.slice(5, 7)), 0)).toISOString().slice(0, 10)
}

/**
 * Signed monthly proof pack (TAQ-20, D-074): the PDF, the copy the scheduler
 * kept, and the export bundle its last page tells the reader to check.
 */
export function ProofPackReport({ site }: { site: Site }) {
  const { t } = useI18n()
  const onError = useDownloadErrorToast()
  const current = monthIn(site.timezone)
  const [month, setMonth] = useState(addMonths(current, -1))
  const [lang, setLang] = useState('')
  const [busy, setBusy] = useState<Busy>(null)
  const valid = /^\d{4}-\d{2}$/.test(month)

  const run = (kind: Exclude<Busy, null>) => async () => {
    setBusy(kind)
    try {
      if (kind === 'bundle') {
        await downloadFile(`/sites/${site.id}/chain/export`, { from: `${month}-01`, to: monthEnd(month) }, `garda-chain-${site.id.split('-')[0]}-${month}-01_${monthEnd(month)}.zip`)
      } else {
        await downloadFile(`/sites/${site.id}/proof-pack`, { month, lang: lang || undefined, stored: kind === 'kept' ? 'true' : undefined }, `proof-pack-${month}.pdf`)
      }
    } catch (err) {
      await onError(err)
    } finally {
      setBusy(null)
    }
  }

  return (
    <div className="max-w-3xl">
      <Toolbar>
        <MonthPicker value={month} onChange={setMonth} current={current} />
        <Field label={t('pp.lang')}>
          <Select value={lang} onChange={(e) => setLang(e.target.value)} className="w-56">
            <option value="">{t('pp.langSite', { lang: site.config_json?.language ?? 'id' })}</option>
            <option value="en">English</option>
            <option value="id">Bahasa Indonesia</option>
          </Select>
        </Field>
      </Toolbar>
      <p className="mb-4 text-sm leading-relaxed text-gray-700">{t('pp.intro', { site: site.name })}</p>
      {month >= current && <Notice tone="info">{t('pp.openMonth')}</Notice>}
      <ul className="mt-4 space-y-4">
        <li className="flex flex-wrap items-center gap-3">
          <Button onClick={run('pdf')} loading={busy === 'pdf'} disabled={!valid}>
            <FileCheck2 className="h-4 w-4" aria-hidden="true" />
            {t('pp.pdf')}
          </Button>
          <span className="text-xs text-gray-500">{t('pp.pdfHint')}</span>
        </li>
        <li className="flex flex-wrap items-center gap-3">
          <Button variant="secondary" onClick={run('kept')} loading={busy === 'kept'} disabled={!valid}>
            <FolderClock className="h-4 w-4" aria-hidden="true" />
            {t('pp.kept')}
          </Button>
          <span className="text-xs text-gray-500">{t('pp.keptHint')}</span>
        </li>
        <li className="flex flex-wrap items-center gap-3">
          <Button variant="secondary" onClick={run('bundle')} loading={busy === 'bundle'} disabled={!valid}>
            <FileArchive className="h-4 w-4" aria-hidden="true" />
            {t('pp.bundle')}
          </Button>
          <span className="text-xs text-gray-500">{t('pp.bundleHint')}</span>
        </li>
      </ul>
      <p className="mt-6 text-sm leading-relaxed text-gray-600">{t('pp.unsigned')}</p>
    </div>
  )
}
