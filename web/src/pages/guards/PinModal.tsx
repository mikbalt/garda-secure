import { useState, type FormEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import apiClient from '../../api/client'
import { Button } from '../../components/ui/Button'
import { Field, FormError, Input } from '../../components/ui/Form'
import { Modal } from '../../components/ui/Modal'
import { useToast } from '../../components/toast/context'
import { useI18n } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import type { Guard } from '../../types'

const PIN_RE = /^[0-9]{4,6}$/

export function PinModal({ guard, onClose }: { guard: Guard; onClose: () => void }) {
  const { t, lang } = useI18n()
  const qc = useQueryClient()
  const toast = useToast()
  const [pin, setPin] = useState('')
  const [repeat, setRepeat] = useState('')
  const [error, setError] = useState('')

  const save = useMutation({
    mutationFn: () => apiClient.put(`/guards/${guard.id}/pin`, { pin }),
    onSuccess: () => {
      toast.success(t('grd.pinSaved'))
      qc.invalidateQueries({ queryKey: ['guards'] })
      onClose()
    },
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!PIN_RE.test(pin)) return setError(t('grd.pinInvalid'))
    if (pin !== repeat) return setError(t('grd.pinMismatch'))
    setError('')
    save.mutate()
  }

  const digits = (v: string) => v.replace(/\D/g, '').slice(0, 6)

  return (
    <Modal
      open
      onClose={onClose}
      title={t('grd.setPinTitle', { name: guard.name })}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="pin-form" loading={save.isPending}>
            {t('common.save')}
          </Button>
        </>
      }
    >
      <form id="pin-form" onSubmit={submit} className="space-y-4" noValidate>
        <FormError message={error || (save.error ? errorText(save.error, lang) : null)} />
        <Field label={t('grd.pin')} required hint={t('grd.pinHint')}>
          <Input type="password" inputMode="numeric" autoComplete="new-password" value={pin} onChange={(e) => setPin(digits(e.target.value))} />
        </Field>
        <Field label={t('grd.pinConfirm')} required>
          <Input type="password" inputMode="numeric" autoComplete="new-password" value={repeat} onChange={(e) => setRepeat(digits(e.target.value))} />
        </Field>
      </form>
    </Modal>
  )
}
