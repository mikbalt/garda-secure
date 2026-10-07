import { useState, type FormEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import apiClient from '../../api/client'
import { Button } from '../../components/ui/Button'
import { Checkbox, Field, FormError, Input, Select } from '../../components/ui/Form'
import { Modal } from '../../components/ui/Modal'
import { Notice } from '../../components/ui/States'
import { useToast } from '../../components/toast/context'
import { useGuards } from '../../hooks/useGuards'
import { useI18n } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import { todayIn, weekdayName } from '../../lib/time'
import type { ShiftTemplate, ShiftTemplateInput, ShiftTemplateResult, Site } from '../../types'

interface Props {
  site: Site
  template: ShiftTemplate | null
  onClose: () => void
}

const DAYS = [1, 2, 3, 4, 5, 6, 7]

export function TemplateModal({ site, template, onClose }: Props) {
  const { t, lang } = useI18n()
  const qc = useQueryClient()
  const toast = useToast()
  const { guards } = useGuards(site.id)
  const activeGuards = guards.filter((g) => g.status === 'active' || g.id === template?.guard_id)

  const [guardId, setGuardId] = useState(template?.guard_id ?? '')
  const [start, setStart] = useState(template?.start_time ?? '07:00')
  const [end, setEnd] = useState(template?.end_time ?? '19:00')
  const [weekdays, setWeekdays] = useState<number[]>(template?.weekdays ?? [1, 2, 3, 4, 5])
  const [validFrom, setValidFrom] = useState(template?.valid_from ?? todayIn(site.timezone))
  const [validUntil, setValidUntil] = useState(template?.valid_until ?? '')
  const [active, setActive] = useState(template?.active ?? true)
  const [formError, setFormError] = useState('')
  const nextDay = !!start && !!end && end <= start

  const save = useMutation({
    mutationFn: async () => {
      const body: ShiftTemplateInput = {
        guard_id: guardId,
        start_time: start,
        end_time: end,
        weekdays: [...weekdays].sort(),
        valid_from: validFrom,
        valid_until: validUntil || null,
        active,
      }
      const res = template
        ? await apiClient.put<ShiftTemplateResult>(`/shift-templates/${template.id}`, body)
        : await apiClient.post<ShiftTemplateResult>('/shift-templates', { ...body, site_id: site.id })
      return res.data
    },
    onSuccess: (res) => {
      toast.success(`${t('common.saved')} ${t('sch.generated', { n: res.generated_shifts })}`)
      qc.invalidateQueries({ queryKey: ['shift-templates', site.id] })
      qc.invalidateQueries({ queryKey: ['shifts', site.id] })
      onClose()
    },
  })

  const toggleDay = (d: number) => setWeekdays((ws) => (ws.includes(d) ? ws.filter((x) => x !== d) : [...ws, d]))

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!guardId || !start || !end || !validFrom) {
      setFormError(t('common.required'))
      return
    }
    if (weekdays.length === 0) {
      setFormError(t('sch.pickWeekday'))
      return
    }
    setFormError('')
    save.mutate()
  }

  return (
    <Modal
      open
      onClose={onClose}
      title={template ? t('sch.editTemplate') : t('sch.newTemplate')}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="template-form" loading={save.isPending}>
            {t('common.save')}
          </Button>
        </>
      }
    >
      <form id="template-form" onSubmit={submit} className="space-y-4" noValidate>
        <FormError message={formError || (save.error ? errorText(save.error, lang) : null)} />
        {activeGuards.length === 0 && <Notice tone="warn">{t('sch.noGuards')}</Notice>}
        <Field label={t('common.guard')} required>
          <Select value={guardId} onChange={(e) => setGuardId(e.target.value)}>
            <option value="">—</option>
            {activeGuards.map((g) => (
              <option key={g.id} value={g.id}>
                {g.name}
                {g.employee_id ? ` (${g.employee_id})` : ''}
              </option>
            ))}
          </Select>
        </Field>
        <div className="grid grid-cols-2 gap-4">
          <Field label={t('sch.start')} required hint={site.timezone}>
            <Input type="time" value={start} onChange={(e) => setStart(e.target.value)} />
          </Field>
          <Field label={t('sch.end')} required hint={nextDay ? t('sch.endsNextDay') : undefined}>
            <Input type="time" value={end} onChange={(e) => setEnd(e.target.value)} />
          </Field>
        </div>
        <fieldset>
          <legend className="mb-1 text-sm font-medium text-gray-800">
            {t('sch.weekdays')} <span className="text-red-700" aria-hidden="true">*</span>
          </legend>
          <div className="flex flex-wrap gap-1.5">
            {DAYS.map((d) => {
              const on = weekdays.includes(d)
              return (
                <button
                  key={d}
                  type="button"
                  aria-pressed={on}
                  onClick={() => toggleDay(d)}
                  className={`min-w-[3rem] rounded-full border px-3 py-1 text-sm focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-600 ${
                    on ? 'border-primary-700 bg-primary-700 text-white' : 'border-gray-300 bg-white text-gray-800 hover:bg-gray-50'
                  }`}
                >
                  {weekdayName(d, lang)}
                </button>
              )
            })}
          </div>
        </fieldset>
        <div className="grid grid-cols-2 gap-4">
          <Field label={t('sch.validFrom')} required>
            <Input type="date" value={validFrom} onChange={(e) => setValidFrom(e.target.value)} />
          </Field>
          <Field label={t('sch.validUntil')} optional>
            <Input type="date" value={validUntil} min={validFrom} onChange={(e) => setValidUntil(e.target.value)} />
          </Field>
        </div>
        <Checkbox label={t('common.active')} checked={active} onChange={(e) => setActive(e.target.checked)} />
      </form>
    </Modal>
  )
}
