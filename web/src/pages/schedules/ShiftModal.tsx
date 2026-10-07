import { useState, type FormEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import apiClient from '../../api/client'
import { Button } from '../../components/ui/Button'
import { Field, FormError, Input, Select } from '../../components/ui/Form'
import { Modal } from '../../components/ui/Modal'
import { Notice } from '../../components/ui/States'
import { useToast } from '../../components/toast/context'
import { useGuards } from '../../hooks/useGuards'
import { useI18n } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import { addDays, localDateOf, localTimeOf, todayIn, zonedToUtcIso } from '../../lib/time'
import type { Shift, Site } from '../../types'

interface Props {
  site: Site
  shift: Shift | null
  defaultDate: string
  onClose: () => void
}

/** Create a one-off shift or edit an existing one (times in the site's zone). */
export function ShiftModal({ site, shift, defaultDate, onClose }: Props) {
  const { t, lang } = useI18n()
  const tz = site.timezone
  const qc = useQueryClient()
  const toast = useToast()
  const { guards } = useGuards(site.id)
  const activeGuards = guards.filter((g) => g.status === 'active' || g.id === shift?.guard_id)

  const today = todayIn(tz)
  const [guardId, setGuardId] = useState(shift?.guard_id ?? '')
  const [date, setDate] = useState(shift ? localDateOf(shift.start_time, tz) : defaultDate < today ? today : defaultDate)
  const [start, setStart] = useState(shift ? localTimeOf(shift.start_time, tz) : '07:00')
  const [end, setEnd] = useState(shift ? localTimeOf(shift.end_time, tz) : '19:00')
  const [formError, setFormError] = useState('')
  const nextDay = !!start && !!end && end <= start

  const save = useMutation({
    mutationFn: () => {
      const body = {
        guard_id: guardId,
        start_time: zonedToUtcIso(date, start, tz),
        end_time: zonedToUtcIso(nextDay ? addDays(date, 1) : date, end, tz),
      }
      return shift ? apiClient.put(`/shifts/${shift.id}`, body) : apiClient.post('/shifts', { ...body, site_id: site.id })
    },
    onSuccess: () => {
      toast.success(t('common.saved'))
      qc.invalidateQueries({ queryKey: ['shifts', site.id] })
      qc.invalidateQueries({ queryKey: ['attendance-daily', site.id] })
      onClose()
    },
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!guardId || !date || !start || !end) {
      setFormError(t('common.required'))
      return
    }
    setFormError('')
    save.mutate()
  }

  return (
    <Modal
      open
      onClose={onClose}
      title={shift ? t('sch.editShift') : t('sch.newShift')}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="shift-form" loading={save.isPending}>
            {t('common.save')}
          </Button>
        </>
      }
    >
      <form id="shift-form" onSubmit={submit} className="space-y-4" noValidate>
        {shift?.template_id && <Notice tone="warn">{t('sch.editDetaches')}</Notice>}
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
        <Field label={t('common.date')} required hint={`${t('common.timezone')}: ${tz}`}>
          <Input type="date" value={date} onChange={(e) => setDate(e.target.value)} />
        </Field>
        <div className="grid grid-cols-2 gap-4">
          <Field label={t('sch.start')} required>
            <Input type="time" value={start} onChange={(e) => setStart(e.target.value)} />
          </Field>
          <Field label={t('sch.end')} required hint={nextDay ? t('sch.endsNextDay') : undefined}>
            <Input type="time" value={end} onChange={(e) => setEnd(e.target.value)} />
          </Field>
        </div>
      </form>
    </Modal>
  )
}
