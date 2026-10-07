import { Field, Input, Select } from '../../components/ui/Form'
import { useI18n } from '../../i18n/context'
import type { IncidentFilterValue } from './incidentFilter'

const SEVERITIES = ['low', 'medium', 'high', 'critical'] as const
const STATUSES = ['open', 'in_progress', 'resolved'] as const

/** Filter controls for incident lists (date range, type, severity, status). */
export function IncidentFilters({
  value,
  onChange,
  knownTypes,
}: {
  value: IncidentFilterValue
  onChange: (v: IncidentFilterValue) => void
  knownTypes: string[]
}) {
  const { t } = useI18n()
  const set = (patch: Partial<IncidentFilterValue>) => onChange({ ...value, ...patch })
  return (
    <>
      <Field label={t('common.from')}>
        <Input type="date" value={value.from} onChange={(e) => set({ from: e.target.value })} />
      </Field>
      <Field label={t('common.to')}>
        <Input type="date" value={value.to} onChange={(e) => set({ to: e.target.value })} />
      </Field>
      <Field label={t('inc.category')}>
        <Input list="incident-types" value={value.type} onChange={(e) => set({ type: e.target.value.toLowerCase() })} className="w-40" />
      </Field>
      <datalist id="incident-types">
        {knownTypes.map((x) => (
          <option key={x} value={x} />
        ))}
      </datalist>
      <Field label={t('severity.label')}>
        <Select value={value.severity} onChange={(e) => set({ severity: e.target.value })}>
          <option value="">{t('common.all')}</option>
          {SEVERITIES.map((s) => (
            <option key={s} value={s}>
              {t(`severity.${s}`)}
            </option>
          ))}
        </Select>
      </Field>
      <Field label={t('common.status')}>
        <Select value={value.status} onChange={(e) => set({ status: e.target.value })}>
          <option value="">{t('common.all')}</option>
          {STATUSES.map((s) => (
            <option key={s} value={s}>
              {t(`status.${s}`)}
            </option>
          ))}
        </Select>
      </Field>
    </>
  )
}
