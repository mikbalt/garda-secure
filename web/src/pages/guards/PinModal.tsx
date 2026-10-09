import { useState, type FormEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { CheckCircle2, Eye, EyeOff, KeyRound } from 'lucide-react'
import apiClient from '../../api/client'
import { Button } from '../../components/ui/Button'
import { Field, FormError, Input } from '../../components/ui/Form'
import { Modal } from '../../components/ui/Modal'
import { useToast } from '../../components/toast/context'
import { useI18n } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import type { Guard } from '../../types'

const PIN_RE = /^[0-9]{4,6}$/

interface Props {
  guard: Guard
  onClose: () => void
  /** The guard was just created: confirm that and frame the PIN as the next step. */
  justCreated?: boolean
}

export function PinModal({ guard, onClose, justCreated = false }: Props) {
  const { t, lang } = useI18n()
  const qc = useQueryClient()
  const toast = useToast()
  const [pin, setPin] = useState('')
  const [repeat, setRepeat] = useState('')
  const [visible, setVisible] = useState(false)
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
  const type = visible ? 'text' : 'password'

  return (
    <Modal
      open
      onClose={onClose}
      title={justCreated ? t('ppl.grd.createdTitle', { name: guard.name }) : guard.has_pin ? t('ppl.grd.changePinTitle', { name: guard.name }) : t('grd.setPinTitle', { name: guard.name })}
      footer={
        <>
          <Button variant={justCreated ? 'ghost' : 'secondary'} onClick={onClose}>
            {justCreated ? t('ppl.grd.later') : t('common.cancel')}
          </Button>
          <Button type="submit" form="pin-form" loading={save.isPending}>
            <KeyRound className="h-4 w-4" aria-hidden="true" />
            {t('ppl.grd.savePin')}
          </Button>
        </>
      }
    >
      <form id="pin-form" onSubmit={submit} className="space-y-4" noValidate>
        {justCreated && (
          <div className="flex items-start gap-2.5 rounded-xl border border-emerald-200 bg-emerald-50 px-3.5 py-2.5 text-sm text-emerald-900">
            <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-emerald-600" aria-hidden="true" />
            <span>{t('ppl.grd.createdNotice', { name: guard.name })}</span>
          </div>
        )}
        <p className="text-sm text-gray-600">{t('ppl.grd.pinIntro')}</p>
        <FormError message={error || (save.error ? errorText(save.error, lang) : null)} />
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label={t('grd.pin')} required hint={t('grd.pinHint')}>
            <Input
              type={type}
              inputMode="numeric"
              autoComplete="new-password"
              value={pin}
              onChange={(e) => setPin(digits(e.target.value))}
              className="font-mono tracking-[0.3em]"
              data-autofocus
            />
          </Field>
          <Field label={t('grd.pinConfirm')} required>
            <Input
              type={type}
              inputMode="numeric"
              autoComplete="new-password"
              value={repeat}
              onChange={(e) => setRepeat(digits(e.target.value))}
              className="font-mono tracking-[0.3em]"
            />
          </Field>
        </div>
        <button
          type="button"
          onClick={() => setVisible((v) => !v)}
          aria-pressed={visible}
          className="inline-flex items-center gap-1.5 rounded text-sm font-medium text-gray-600 hover:text-gray-900 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
        >
          {visible ? <EyeOff className="h-4 w-4" aria-hidden="true" /> : <Eye className="h-4 w-4" aria-hidden="true" />}
          {visible ? t('ppl.grd.pinHide') : t('ppl.grd.pinShow')}
        </button>
      </form>
    </Modal>
  )
}
