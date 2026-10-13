import { useState, type FormEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { CheckCircle2, Mail, TriangleAlert, UserPlus } from 'lucide-react'
import apiClient, { apiErrorField } from '../../api/client'
import { SecretOnce } from '../../components/SecretOnce'
import { Button } from '../../components/ui/Button'
import { Field, FormError, Input } from '../../components/ui/Form'
import { Modal } from '../../components/ui/Modal'
import { useToast } from '../../components/toast/context'
import { useSite } from '../../context/site'
import { useI18n } from '../../i18n/context'
import { ERROR_CODES, errorText } from '../../i18n/errors'
import type { ManagedUser, PasswordResetResult, UserCreateInput, UserCreateResult } from '../../types'
import { RolePicker, SiteScopePicker } from './UserFields'
import { assignableRoles, displayName, emailError, normaliseUsername, sitesError, usernameError } from './userRules'

type Errors = Partial<Record<'username' | 'email' | 'roles' | 'sites' | 'temporary_password', string>>

/** Fields the API reports in `field` that this form shows inline. */
const FORM_FIELDS = ['username', 'email', 'roles', 'sites', 'temporary_password'] as const

interface Props {
  callerRoles: readonly string[]
  callerRestricted: boolean
  onClose: () => void
}

/** New web user: identity, roles, site scope and how the first password is delivered. */
export function UserCreateModal({ callerRoles, callerRestricted, onClose }: Props) {
  const { t, lang } = useI18n()
  const { sites } = useSite()
  const qc = useQueryClient()
  const toast = useToast()
  const [username, setUsername] = useState('')
  const [firstName, setFirstName] = useState('')
  const [lastName, setLastName] = useState('')
  const [email, setEmail] = useState('')
  const [roles, setRoles] = useState<string[]>([])
  const [siteIds, setSiteIds] = useState<string[]>([])
  const [delivery, setDelivery] = useState<'invite' | 'password'>('invite')
  const [ownPassword, setOwnPassword] = useState('')
  const [errors, setErrors] = useState<Errors>({})
  const [done, setDone] = useState<{ user: ManagedUser; password?: string; inviteFailed?: boolean } | null>(null)

  const msg = (code: string) => ERROR_CODES[code]?.[lang] ?? code

  const create = useMutation({
    mutationFn: (body: UserCreateInput) => apiClient.post<UserCreateResult>('/users', body).then((r) => r.data),
    onSuccess: (res) => {
      qc.invalidateQueries({ queryKey: ['users'] })
      if (res.invite_sent) {
        toast.success(t('acc.usr.invited', { email: res.user.email }))
        onClose()
        return
      }
      setDone({ user: res.user, password: res.temporary_password, inviteFailed: res.invite_error === 'invite_failed' })
    },
    onError: (err) => {
      const field = apiErrorField(err)
      if (field && (FORM_FIELDS as readonly string[]).includes(field)) setErrors({ [field]: errorText(err, lang) })
    },
  })

  // Invitation failed: hand out a temporary password instead.
  const fallback = useMutation({
    mutationFn: (id: string) => apiClient.post<PasswordResetResult>(`/users/${id}/reset-password`, {}).then((r) => r.data),
    onSuccess: (res) => done && setDone({ ...done, password: res.temporary_password, inviteFailed: false }),
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const next: Errors = {}
    const u = usernameError(username)
    if (u) next.username = msg(u)
    const em = emailError(email, delivery === 'invite')
    if (em) next.email = msg(em)
    if (roles.length === 0) next.roles = msg('roles_required')
    const s = sitesError(siteIds, callerRestricted)
    if (s) next.sites = msg(s)
    setErrors(next)
    if (Object.keys(next).length) return
    create.mutate({
      username: normaliseUsername(username),
      email: email.trim() || undefined,
      first_name: firstName.trim() || undefined,
      last_name: lastName.trim() || undefined,
      roles,
      sites: siteIds,
      send_invite: delivery === 'invite',
      temporary_password: delivery === 'password' && ownPassword ? ownPassword : undefined,
    })
  }

  if (done) {
    const name = displayName(done.user)
    return (
      <Modal
        open
        onClose={onClose}
        title={t('acc.usr.createdTitle', { name })}
        footer={
          <Button onClick={onClose} disabled={fallback.isPending}>
            {t('acc.secret.done')}
          </Button>
        }
      >
        <div className="space-y-4">
          <div className="flex items-start gap-2.5 rounded-xl border border-emerald-200 bg-emerald-50 px-3.5 py-2.5 text-sm text-emerald-900">
            <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-emerald-600" aria-hidden="true" />
            <span>{t('acc.usr.createdBody', { username: done.user.username })}</span>
          </div>
          {done.inviteFailed && !done.password && (
            <div className="space-y-3">
              <div className="flex items-start gap-2.5 rounded-xl border border-amber-200 bg-amber-50 px-3.5 py-2.5 text-sm text-amber-900">
                <TriangleAlert className="mt-0.5 h-4 w-4 shrink-0 text-amber-600" aria-hidden="true" />
                <span>{t('acc.usr.inviteFailed')}</span>
              </div>
              <FormError message={fallback.error ? errorText(fallback.error, lang) : null} />
              <Button variant="secondary" loading={fallback.isPending} onClick={() => fallback.mutate(done.user.id)}>
                {t('acc.usr.setTempInstead')}
              </Button>
            </div>
          )}
          {done.password && (
            <SecretOnce label={t('acc.usr.tempPassword')} value={done.password}>
              <p className="text-sm text-gray-600">{t('acc.usr.tempPasswordNext', { username: done.user.username })}</p>
            </SecretOnce>
          )}
        </div>
      </Modal>
    )
  }

  const available = assignableRoles(callerRoles)
  // A general error unless it is shown next to its field.
  const field = create.error ? apiErrorField(create.error) : undefined
  const general = create.error && !(field && (FORM_FIELDS as readonly string[]).includes(field)) ? errorText(create.error, lang) : null

  return (
    <Modal
      open
      onClose={onClose}
      size="lg"
      title={t('acc.usr.createTitle')}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="user-create-form" loading={create.isPending}>
            <UserPlus className="h-4 w-4" aria-hidden="true" />
            {t('acc.usr.create')}
          </Button>
        </>
      }
    >
      <form id="user-create-form" onSubmit={submit} className="space-y-5" noValidate>
        <p className="text-sm text-gray-600">{t('acc.usr.createIntro')}</p>
        <FormError message={general} />
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label={t('acc.usr.username')} required hint={t('acc.usr.usernameHint')} error={errors.username} className="sm:col-span-2">
            <Input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="off" spellCheck={false} maxLength={64} />
          </Field>
          <Field label={t('acc.usr.firstName')} optional>
            <Input value={firstName} onChange={(e) => setFirstName(e.target.value)} autoComplete="off" maxLength={200} />
          </Field>
          <Field label={t('acc.usr.lastName')} optional>
            <Input value={lastName} onChange={(e) => setLastName(e.target.value)} autoComplete="off" maxLength={200} />
          </Field>
          <Field
            label={t('acc.usr.email')}
            required={delivery === 'invite'}
            optional={delivery !== 'invite'}
            hint={delivery === 'invite' ? t('acc.usr.emailInviteHint') : t('acc.usr.emailHint')}
            error={errors.email}
            className="sm:col-span-2"
          >
            <Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="off" maxLength={254} />
          </Field>
        </div>

        <RolePicker available={available} value={roles} onChange={setRoles} error={errors.roles} />
        <SiteScopePicker sites={sites} value={siteIds} onChange={setSiteIds} restricted={callerRestricted} error={errors.sites} />

        <fieldset>
          <legend className="mb-1.5 block text-sm font-medium text-gray-700">{t('acc.usr.delivery')}</legend>
          <div className="grid gap-2 sm:grid-cols-2">
            {(['invite', 'password'] as const).map((d) => (
              <label
                key={d}
                className={`flex cursor-pointer items-start gap-2.5 rounded-xl border p-3 text-sm ${delivery === d ? 'border-primary-300 bg-primary-50/50' : 'border-gray-200 hover:bg-gray-50'}`}
              >
                <input
                  type="radio"
                  name="user-delivery"
                  className="mt-0.5 h-4 w-4 border-gray-300 text-primary-600 focus:ring-2 focus:ring-primary-500"
                  checked={delivery === d}
                  onChange={() => setDelivery(d)}
                />
                <span>
                  <span className="flex items-center gap-1.5 font-medium text-gray-900">
                    {d === 'invite' && <Mail className="h-3.5 w-3.5 text-gray-500" aria-hidden="true" />}
                    {t(d === 'invite' ? 'acc.usr.deliveryInvite' : 'acc.usr.deliveryPassword')}
                  </span>
                  <span className="mt-0.5 block text-xs leading-snug text-gray-600">
                    {t(d === 'invite' ? 'acc.usr.deliveryInviteHint' : 'acc.usr.deliveryPasswordHint')}
                  </span>
                </span>
              </label>
            ))}
          </div>
        </fieldset>
        {delivery === 'password' && (
          <Field label={t('acc.usr.ownPassword')} optional hint={t('acc.usr.ownPasswordHint')} error={errors.temporary_password}>
            <Input type="text" value={ownPassword} onChange={(e) => setOwnPassword(e.target.value)} autoComplete="new-password" spellCheck={false} maxLength={128} />
          </Field>
        )}
        <p className="text-xs text-gray-500">{t('acc.usr.otpNote')}</p>
      </form>
    </Modal>
  )
}
