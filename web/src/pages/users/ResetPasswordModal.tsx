import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { KeyRound, Mail } from 'lucide-react'
import apiClient from '../../api/client'
import { SecretOnce } from '../../components/SecretOnce'
import { Button } from '../../components/ui/Button'
import { Field, FormError, Input } from '../../components/ui/Form'
import { Modal } from '../../components/ui/Modal'
import { useToast } from '../../components/toast/context'
import { useI18n } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import type { ManagedUser, PasswordResetResult } from '../../types'
import { displayName } from './userRules'

/** Reset a user's password: e-mail a reset link, or set a temporary password shown once. */
export function ResetPasswordModal({ user, onClose }: { user: ManagedUser; onClose: () => void }) {
  const { t, lang } = useI18n()
  const toast = useToast()
  const hasEmail = !!user.email
  const [mode, setMode] = useState<'email' | 'temporary'>(hasEmail ? 'email' : 'temporary')
  const [own, setOwn] = useState('')
  const [password, setPassword] = useState<string | null>(null)
  const name = displayName(user)

  const reset = useMutation({
    mutationFn: () =>
      apiClient
        .post<PasswordResetResult>(`/users/${user.id}/reset-password`, mode === 'email' ? { send_email: true } : own ? { temporary_password: own } : {})
        .then((r) => r.data),
    onSuccess: (res) => {
      if (res.email_sent) {
        toast.success(t('acc.usr.resetEmailSent', { email: user.email }))
        onClose()
      } else if (res.temporary_password) {
        setPassword(res.temporary_password)
      }
    },
  })

  if (password) {
    return (
      <Modal open onClose={onClose} title={t('acc.usr.resetDoneTitle', { name })} footer={<Button onClick={onClose}>{t('acc.secret.done')}</Button>}>
        <SecretOnce label={t('acc.usr.tempPassword')} value={password}>
          <p className="text-sm text-gray-600">{t('acc.usr.tempPasswordNext', { username: user.username })}</p>
        </SecretOnce>
      </Modal>
    )
  }

  return (
    <Modal
      open
      onClose={onClose}
      title={t('acc.usr.resetTitle', { name })}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button onClick={() => reset.mutate()} loading={reset.isPending}>
            <KeyRound className="h-4 w-4" aria-hidden="true" />
            {t('acc.usr.resetConfirm')}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <p className="text-sm text-gray-600">{t('acc.usr.resetIntro')}</p>
        <FormError message={reset.error ? errorText(reset.error, lang) : null} />
        <fieldset>
          <legend className="sr-only">{t('acc.usr.resetHow')}</legend>
          <div className="space-y-2">
            <label className={`flex items-start gap-2.5 rounded-xl border p-3 text-sm ${!hasEmail ? 'opacity-60' : 'cursor-pointer'} ${mode === 'email' ? 'border-primary-300 bg-primary-50/50' : 'border-gray-200'}`}>
              <input
                type="radio"
                name="reset-mode"
                className="mt-0.5 h-4 w-4 border-gray-300 text-primary-600 focus:ring-2 focus:ring-primary-500"
                checked={mode === 'email'}
                disabled={!hasEmail}
                onChange={() => setMode('email')}
              />
              <span>
                <span className="flex items-center gap-1.5 font-medium text-gray-900">
                  <Mail className="h-3.5 w-3.5 text-gray-500" aria-hidden="true" />
                  {t('acc.usr.resetByEmail')}
                </span>
                <span className="mt-0.5 block text-xs text-gray-600">
                  {hasEmail ? t('acc.usr.resetByEmailHint', { email: user.email }) : t('acc.usr.resetNoEmail')}
                </span>
              </span>
            </label>
            <label className={`flex cursor-pointer items-start gap-2.5 rounded-xl border p-3 text-sm ${mode === 'temporary' ? 'border-primary-300 bg-primary-50/50' : 'border-gray-200'}`}>
              <input
                type="radio"
                name="reset-mode"
                className="mt-0.5 h-4 w-4 border-gray-300 text-primary-600 focus:ring-2 focus:ring-primary-500"
                checked={mode === 'temporary'}
                onChange={() => setMode('temporary')}
              />
              <span>
                <span className="font-medium text-gray-900">{t('acc.usr.resetTemporary')}</span>
                <span className="mt-0.5 block text-xs text-gray-600">{t('acc.usr.resetTemporaryHint')}</span>
              </span>
            </label>
          </div>
        </fieldset>
        {mode === 'temporary' && (
          <Field label={t('acc.usr.ownPassword')} optional hint={t('acc.usr.ownPasswordHint')}>
            <Input type="text" value={own} onChange={(e) => setOwn(e.target.value)} autoComplete="new-password" spellCheck={false} maxLength={128} />
          </Field>
        )}
      </div>
    </Modal>
  )
}
