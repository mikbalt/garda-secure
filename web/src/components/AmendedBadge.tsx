import { FilePen } from 'lucide-react'
import { useI18n } from '../i18n/context'
import { AMENDABLE, amendedFields } from '../lib/incidents'
import type { Incident } from '../types'

/**
 * "Amended" pill (D-043): the shown values include a supervisor's
 * correction; the filed original is kept and shown in the incident detail.
 * Static (no button) so it can sit inside clickable rows.
 */
export function AmendedBadge({ incident }: { incident: Pick<Incident, 'original_type' | 'original_severity' | 'original_location' | 'original_description'> }) {
  const { t } = useI18n()
  const fields = amendedFields(incident).map((f) => t(AMENDABLE.find((a) => a.field === f)!.label))
  const title = fields.length ? t('p2.inc.amendedFields', { fields: fields.join(', ') }) : t('p2.inc.amendedHint')
  return (
    <span title={title} className="inline-flex items-center gap-1 whitespace-nowrap rounded-full bg-violet-50 px-2 py-0.5 text-xs font-medium text-violet-700 ring-1 ring-inset ring-violet-200">
      <FilePen className="h-3 w-3" aria-hidden="true" />
      {t('p2.inc.amended')}
      <span className="sr-only">: {title}</span>
    </span>
  )
}
