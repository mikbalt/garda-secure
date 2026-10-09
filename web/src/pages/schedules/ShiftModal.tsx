import { useState, type FormEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { ClipboardCheck, Trash2 } from 'lucide-react'
import apiClient, { apiErrorCode } from '../../api/client'
import { Button } from '../../components/ui/Button'
import { Field, FormError, Input, Select } from '../../components/ui/Form'
import { Modal } from '../../components/ui/Modal'
import { Notice } from '../../components/ui/States'
import { buttonClass } from '../../components/ui/styles'
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
  defaultGuardId?: string
  onClose: () => void
}

/** Create a one-off shift or edit / delete an existing one (times in the site's zone). */
export function ShiftModal({ site, shift, defaultDate, defaultGuardId, onClose }: Props) {
  const { t, lang } = useI18n()
  const tz = site.timezone
  const qc = useQueryClient()
  const toast = useToast()
  const { guards } = useGuards(site.id)
  const activeGuards = guards.filter((g) => g.status === 'active' || g.id === shift?.guard_id)

  const today = todayIn(tz)
  const [guardId, setGuardId] = useState(shift?.guard_id ?? defaultGuardId ?? '')
  const [date, setDate] = useState(shift ? localDateOf(shift.start_time, tz) : defaultDate < today ? today : defaultDate)
  const [start, setStart] = useState(shift ? localTimeOf(shift.start_time, tz) : '07:00')
  const [end, setEnd] = useState(shift ? localTimeOf(shift.end_time, tz) : '15:00')
  const [formError, setFormError] = useState('')
  const [confirmDelete, setConfirmDelete] = useState(false)
  const nextDay = !!start && !!end && end <= start

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ['shifts', site.id] })
    qc.invalidateQueries({ queryKey: ['attendance-daily', site.id] })
    qc.invalidateQueries({ queryKey: ['dashboard', site.id] })
  }

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
      invalidate()
      onClose()
    },
  })

  const del = useMutation({
    mutationFn: () => apiClient.delete(`/shifts/${shift!.id}`),
    onSuccess: () => {
      toast.success(t('ppl.sch.shiftDeleted'))
      invalidate()
      onClose()
    },
    onSettled: () => setConfirmDelete(false),
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!guardId || !date || !start || !end) {
      setFormError(t('common.required'))
      return
    }
    setFormError('')
    del.reset()
    save.mutate()
  }

  const failure = save.error ?? del.error
  const hasAttendance = apiErrorCode(failure) === 'shift_has_attendance'
  const shiftDay = shift ? localDateOf(shift.start_time, tz) : date

  return (
    <Modal
      open
      onClose={onClose}
      title={shift ? t('sch.editShift') : t('ppl.sch.newOneOff')}
      footer={
        confirmDelete ? (
          <>
            <span className="mr-auto self-center text-sm font-medium text-gray-900">{t('ppl.sch.confirmDeleteShift')}</span>
            <Button variant="secondary" onClick={() => setConfirmDelete(false)}>
              {t('common.cancel')}
            </Button>
            <Button variant="danger" loading={del.isPending} onClick={() => del.mutate()} autoFocus>
              {t('ppl.sch.yesDelete')}
            </Button>
          </>
        ) : (
          <>
            {shift && (
              <Button
                variant="ghost"
                className="mr-auto text-red-700 hover:bg-red-50 hover:text-red-800"
                onClick={() => {
                  save.reset()
                  setConfirmDelete(true)
                }}
              >
                <Trash2 className="h-4 w-4" aria-hidden="true" />
                <span className="hidden sm:inline">{t('ppl.sch.deleteShift')}</span>
                <span className="sr-only sm:hidden">{t('ppl.sch.deleteShift')}</span>
              </Button>
            )}
            <Button variant="secondary" onClick={onClose}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" form="shift-form" loading={save.isPending}>
              {t('common.save')}
            </Button>
          </>
        )
      }
    >
      <form id="shift-form" onSubmit={submit} className="space-y-4" noValidate>
        {!shift && <p className="text-sm text-gray-600">{t('ppl.sch.oneOffIntro')}</p>}
        {shift?.template_id && <Notice tone="warn">{t('ppl.sch.editDetaches')}</Notice>}
        {hasAttendance ? (
          <div role="alert" className="rounded-xl border border-amber-200 bg-amber-50 px-3.5 py-3 text-sm text-amber-900">
            <p>{t('ppl.sch.hasAttendance')}</p>
            <Link to={`/attendance?date=${shiftDay}`} className={buttonClass('secondary', 'sm', 'mt-2.5')}>
              <ClipboardCheck className="h-3.5 w-3.5" aria-hidden="true" />
              {t('ppl.sch.openAttendance')}
            </Link>
          </div>
        ) : (
          <FormError message={formError || (failure ? errorText(failure, lang) : null)} />
        )}
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
        <Field label={t('common.date')} required hint={t('ppl.sch.timesIn', { tz })}>
          <Input type="date" value={date} min={shift ? undefined : today} onChange={(e) => setDate(e.target.value)} />
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
