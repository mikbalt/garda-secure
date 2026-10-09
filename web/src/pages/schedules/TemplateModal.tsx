import { useState, type FormEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Moon } from 'lucide-react'
import apiClient from '../../api/client'
import { Button } from '../../components/ui/Button'
import { Checkbox, Field, FormError, Input, Select } from '../../components/ui/Form'
import { Modal } from '../../components/ui/Modal'
import { Notice } from '../../components/ui/States'
import { useToast } from '../../components/toast/context'
import { useGuards } from '../../hooks/useGuards'
import { useI18n, type Key } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import { todayIn, weekdayName } from '../../lib/time'
import { periodDot, periodOf } from '../guards/peopleUtil'
import type { ShiftTemplate, ShiftTemplateInput, ShiftTemplateResult, Site } from '../../types'

interface Props {
  site: Site
  template: ShiftTemplate | null
  onClose: () => void
}

const DAYS = [1, 2, 3, 4, 5, 6, 7]

/** Common Indonesian guard shifts: 3 × 8 h and 2 × 12 h. */
const TIME_PRESETS: [string, string][] = [
  ['07:00', '15:00'],
  ['15:00', '23:00'],
  ['23:00', '07:00'],
  ['07:00', '19:00'],
  ['19:00', '07:00'],
]

const DAY_PRESETS: { key: Key; days: number[] }[] = [
  { key: 'ppl.sch.everyDay', days: [1, 2, 3, 4, 5, 6, 7] },
  { key: 'ppl.sch.monFri', days: [1, 2, 3, 4, 5] },
  { key: 'ppl.sch.monSat', days: [1, 2, 3, 4, 5, 6] },
  { key: 'ppl.sch.weekend', days: [6, 7] },
]

const sameDays = (a: number[], b: number[]) => a.length === b.length && a.every((d) => b.includes(d))

const chipBase =
  'inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-medium ring-1 ring-inset transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500'
const chipOn = 'bg-primary-50 text-primary-800 ring-primary-300'
const chipOff = 'bg-white text-gray-700 ring-gray-200 hover:bg-gray-50'

export function TemplateModal({ site, template, onClose }: Props) {
  const { t, lang } = useI18n()
  const qc = useQueryClient()
  const toast = useToast()
  const { guards } = useGuards(site.id)
  const activeGuards = guards.filter((g) => g.status === 'active' || g.id === template?.guard_id)

  const [guardId, setGuardId] = useState(template?.guard_id ?? '')
  const [start, setStart] = useState(template?.start_time ?? '07:00')
  const [end, setEnd] = useState(template?.end_time ?? '15:00')
  const [weekdays, setWeekdays] = useState<number[]>(template?.weekdays ?? [1, 2, 3, 4, 5, 6])
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
      qc.invalidateQueries({ queryKey: ['attendance-daily', site.id] })
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
      size="lg"
      title={template ? t('ppl.sch.editPattern') : t('ppl.sch.newPattern')}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="template-form" loading={save.isPending}>
            {template ? t('common.save') : t('ppl.sch.savePattern')}
          </Button>
        </>
      }
    >
      <form id="template-form" onSubmit={submit} className="space-y-5" noValidate>
        <p className="text-sm text-gray-600">{template ? t('ppl.sch.editPatternIntro') : t('ppl.sch.patternIntro')}</p>
        <FormError message={formError || (save.error ? errorText(save.error, lang) : null)} />
        {activeGuards.length === 0 && <Notice tone="warn">{t('sch.noGuards')}</Notice>}
        <Field label={t('common.guard')} required>
          <Select value={guardId} onChange={(e) => setGuardId(e.target.value)}>
            <option value="">{t('ppl.sch.pickGuard')}</option>
            {activeGuards.map((g) => (
              <option key={g.id} value={g.id}>
                {g.name}
                {g.employee_id ? ` (${g.employee_id})` : ''}
              </option>
            ))}
          </Select>
        </Field>

        <fieldset className="space-y-2.5">
          <legend className="mb-1.5 block text-sm font-medium text-gray-700">
            {t('ppl.sch.hours')} <span className="text-red-700" aria-hidden="true">*</span>
          </legend>
          <div className="flex flex-wrap gap-1.5" role="group" aria-label={t('ppl.sch.quickTimes')}>
            {TIME_PRESETS.map(([s, e]) => {
              const on = s === start && e === end
              return (
                <button key={`${s}-${e}`} type="button" aria-pressed={on} onClick={() => { setStart(s); setEnd(e) }} className={`${chipBase} ${on ? chipOn : chipOff}`}>
                  <span className={`h-1.5 w-1.5 rounded-full ${periodDot[periodOf(s)]}`} aria-hidden="true" />
                  <span className="tabular-nums">
                    {s}–{e}
                  </span>
                </button>
              )
            })}
          </div>
          <div className="grid grid-cols-2 gap-4">
            <Field label={t('sch.start')} hint={t('ppl.sch.timesIn', { tz: site.timezone })}>
              <Input type="time" value={start} onChange={(e) => setStart(e.target.value)} />
            </Field>
            <Field
              label={t('sch.end')}
              hint={
                nextDay ? (
                  <span className="inline-flex items-center gap-1 font-medium text-indigo-700">
                    <Moon className="h-3 w-3" aria-hidden="true" />
                    {t('sch.endsNextDay')}
                  </span>
                ) : undefined
              }
            >
              <Input type="time" value={end} onChange={(e) => setEnd(e.target.value)} />
            </Field>
          </div>
        </fieldset>

        <fieldset>
          <legend className="mb-1.5 block text-sm font-medium text-gray-700">
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
                  className={`h-9 min-w-[3.25rem] rounded-lg px-3 text-sm font-medium transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-1 ${
                    on ? 'bg-primary-600 text-white shadow-sm hover:bg-primary-700' : 'bg-white text-gray-700 ring-1 ring-inset ring-gray-300 hover:bg-gray-50'
                  }`}
                >
                  {weekdayName(d, lang)}
                </button>
              )
            })}
          </div>
          <div className="mt-2.5 flex flex-wrap items-center gap-1.5">
            <span className="mr-1 text-xs text-gray-500">{t('ppl.sch.quickPick')}</span>
            {DAY_PRESETS.map((p) => {
              const on = sameDays(weekdays, p.days)
              return (
                <button key={p.key} type="button" aria-pressed={on} onClick={() => setWeekdays(p.days)} className={`${chipBase} ${on ? chipOn : chipOff}`}>
                  {t(p.key)}
                </button>
              )
            })}
          </div>
        </fieldset>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label={t('sch.validFrom')} required>
            <Input type="date" value={validFrom} onChange={(e) => setValidFrom(e.target.value)} />
          </Field>
          <Field label={t('sch.validUntil')} optional hint={t('ppl.sch.validUntilHint')}>
            <Input type="date" value={validUntil} min={validFrom} onChange={(e) => setValidUntil(e.target.value)} />
          </Field>
        </div>
        <div>
          <Checkbox label={t('ppl.sch.activeLabel')} checked={active} onChange={(e) => setActive(e.target.checked)} />
          <p className="ml-6 mt-0.5 text-xs text-gray-500">{t('ppl.sch.activeHint')}</p>
        </div>
      </form>
    </Modal>
  )
}
