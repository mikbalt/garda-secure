import { useState, type FormEvent, type ReactNode } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { CircleUserRound, KeyRound, LogOut, ShieldCheck, ShieldOff, Smartphone, UserCheck } from 'lucide-react'
import apiClient, { apiErrorField } from '../../api/client'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Field, FormError, Input } from '../../components/ui/Form'
import { Modal } from '../../components/ui/Modal'
import { Notice } from '../../components/ui/States'
import { useToast } from '../../components/toast/context'
import { useSite } from '../../context/site'
import { useI18n } from '../../i18n/context'
import { ERROR_CODES, errorText } from '../../i18n/errors'
import { formatDateTime } from '../../lib/time'
import { Avatar } from '../guards/PeopleKit'
import { Switch } from '../settings/kit'
import type { ManagedUser } from '../../types'
import { RolePicker, SiteScopePicker } from './UserFields'
import { assignableRoles, diffUser, displayName, emailError, isManageable, lockedOwnRoles, otpMissing, pendingActions, requiredActionLabel, selfUpdateError, sitesError } from './userRules'

export type UserAction = 'reset-password' | 'reset-otp' | 'logout'

interface Props {
  user: ManagedUser
  isSelf: boolean
  callerRoles: readonly string[]
  callerRestricted: boolean
  onClose: () => void
  /** Opens a follow-up dialog (the drawer closes first: one dialog at a time). */
  onAction: (action: UserAction, user: ManagedUser) => void
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="space-y-4 border-t border-gray-100 pt-5 first:border-0 first:pt-0">
      <h3 className="text-[15px] font-semibold text-gray-900">{title}</h3>
      {children}
    </section>
  )
}

/** Details, edit form and sign-in actions of one user (admin). */
export function UserDrawer({ user, isSelf, callerRoles, callerRestricted, onClose, onAction }: Props) {
  const { t, lang } = useI18n()
  const { sites, site } = useSite()
  const qc = useQueryClient()
  const toast = useToast()
  const tz = site?.timezone ?? 'Asia/Jakarta'
  const manageable = isManageable(user, callerRoles)
  const [firstName, setFirstName] = useState(user.first_name)
  const [lastName, setLastName] = useState(user.last_name)
  const [email, setEmail] = useState(user.email)
  const [enabled, setEnabled] = useState(user.enabled)
  const [roles, setRoles] = useState<string[]>(user.roles)
  const [siteIds, setSiteIds] = useState<string[]>(user.sites)
  const [errors, setErrors] = useState<Partial<Record<'email' | 'roles' | 'sites', string>>>({})
  const msg = (code: string) => ERROR_CODES[code]?.[lang] ?? code

  const available = assignableRoles(callerRoles)
  // Roles the user holds but the caller cannot grant stay visible (read-only).
  const shownRoles = [...available, ...user.roles.filter((r) => !(available as readonly string[]).includes(r))]

  const save = useMutation({
    mutationFn: (patch: ReturnType<typeof diffUser>) => apiClient.put<ManagedUser>(`/users/${user.id}`, patch).then((r) => r.data),
    onSuccess: () => {
      toast.success(t('common.saved'))
      qc.invalidateQueries({ queryKey: ['users'] })
      onClose()
    },
    onError: (err) => {
      const f = apiErrorField(err)
      if (f === 'email' || f === 'roles' || f === 'sites') setErrors({ [f]: errorText(err, lang) })
    },
  })
  const enable = useMutation({
    mutationFn: () => apiClient.put<ManagedUser>(`/users/${user.id}`, { enabled: true }),
    onSuccess: () => {
      toast.success(t('acc.usr.enabledToast', { name: displayName(user) }))
      qc.invalidateQueries({ queryKey: ['users'] })
      setEnabled(true)
    },
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const next: typeof errors = {}
    const em = emailError(email, false)
    if (em) next.email = msg(em)
    if (roles.length === 0) next.roles = msg('roles_required')
    const s = sitesError(siteIds, callerRestricted)
    if (s) next.sites = msg(s)
    setErrors(next)
    if (Object.keys(next).length) return
    const patch = diffUser(user, { first_name: firstName, last_name: lastName, email, enabled, roles, sites: siteIds })
    if (Object.keys(patch).length === 0) return onClose()
    if (selfUpdateError(user, patch, isSelf)) {
      setErrors({ roles: msg('cannot_modify_self') })
      return
    }
    save.mutate(patch)
  }

  const general = save.error && !['email', 'roles', 'sites'].includes(apiErrorField(save.error) ?? '') ? errorText(save.error, lang) : null
  const actions = pendingActions(user)
  const name = displayName(user)

  return (
    <Modal
      open
      variant="drawer"
      onClose={onClose}
      title={t('acc.usr.drawerTitle', { name })}
      footer={
        manageable ? (
          <>
            <Button variant="secondary" onClick={onClose}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" form="user-edit-form" loading={save.isPending}>
              {t('common.save')}
            </Button>
          </>
        ) : (
          <Button variant="secondary" onClick={onClose}>
            {t('common.close')}
          </Button>
        )
      }
    >
      <div className="space-y-6">
        <div className="flex items-start gap-3 rounded-xl border border-gray-200 bg-gray-50/70 p-3.5">
          <Avatar name={name} size="lg" />
          <div className="min-w-0 flex-1">
            <p className="flex flex-wrap items-center gap-2 text-sm font-semibold text-gray-900">
              <span className="truncate">{name}</span>
              {isSelf && <Badge tone="blue">{t('acc.usr.you')}</Badge>}
              {user.enabled ? <Badge tone="green" dot>{t('acc.usr.enabled')}</Badge> : <Badge tone="red" dot>{t('acc.usr.disabled')}</Badge>}
            </p>
            <p className="truncate text-xs text-gray-600">
              <span className="font-mono">{user.username}</span>
              {user.email ? ` · ${user.email}` : ''}
            </p>
            <p className="mt-0.5 text-xs text-gray-600">{t('acc.usr.createdAt', { when: formatDateTime(user.created_at, tz, lang) })}</p>
          </div>
        </div>

        {!manageable && <Notice tone="warn">{t('acc.usr.platformOnly')}</Notice>}
        {manageable && !user.enabled && (
          <div className="flex flex-wrap items-center gap-3 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-900">
            <ShieldOff className="h-4 w-4 shrink-0 text-red-600" aria-hidden="true" />
            <p className="min-w-0 flex-1">{t('acc.usr.disabledNotice')}</p>
            <Button size="sm" variant="secondary" loading={enable.isPending} onClick={() => enable.mutate()}>
              <UserCheck className="h-3.5 w-3.5" aria-hidden="true" />
              {t('acc.usr.enable')}
            </Button>
          </div>
        )}
        <FormError message={enable.error ? errorText(enable.error, lang) : null} />

        <form id="user-edit-form" onSubmit={submit} noValidate className="space-y-6">
          <Section title={t('acc.usr.profile')}>
            <FormError message={general} />
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label={t('acc.usr.firstName')} optional>
                <Input value={firstName} onChange={(e) => setFirstName(e.target.value)} disabled={!manageable} maxLength={200} autoComplete="off" />
              </Field>
              <Field label={t('acc.usr.lastName')} optional>
                <Input value={lastName} onChange={(e) => setLastName(e.target.value)} disabled={!manageable} maxLength={200} autoComplete="off" />
              </Field>
              <Field label={t('acc.usr.email')} optional error={errors.email} hint={t('acc.usr.emailHint')} className="sm:col-span-2">
                <Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} disabled={!manageable} maxLength={254} autoComplete="off" />
              </Field>
            </div>
          </Section>

          <Section title={t('acc.usr.access')}>
            <RolePicker available={shownRoles} value={roles} onChange={setRoles} locked={lockedOwnRoles(user, isSelf)} disabled={!manageable} error={errors.roles} />
            <SiteScopePicker
              sites={sites}
              value={siteIds}
              onChange={setSiteIds}
              restricted={callerRestricted}
              disabled={!manageable || isSelf}
              error={errors.sites}
            />
            {isSelf && <p className="text-xs text-gray-500">{t('acc.usr.ownSitesHint')}</p>}
            <div className="flex items-start justify-between gap-4 rounded-xl border border-gray-200 p-3.5">
              <div className="min-w-0">
                <p className="text-sm font-medium text-gray-900">{t('acc.usr.accountEnabled')}</p>
                <p className="text-xs text-gray-600">{isSelf ? t('acc.usr.cannotDisableSelf') : t('acc.usr.accountEnabledHint')}</p>
              </div>
              <Switch checked={enabled} onChange={setEnabled} label={t('acc.usr.accountEnabled')} disabled={!manageable || isSelf} />
            </div>
          </Section>
        </form>

        <Section title={t('acc.usr.signIn')}>
          <ul className="space-y-2 text-sm">
            <li className="flex items-center gap-2">
              <Smartphone className="h-4 w-4 text-gray-500" aria-hidden="true" />
              {user.otp_configured ? (
                <Badge tone="green">
                  <ShieldCheck className="h-3 w-3" aria-hidden="true" />
                  {t('acc.usr.otpOn')}
                </Badge>
              ) : otpMissing(user) ? (
                <Badge tone="yellow">{t('acc.usr.otpPending')}</Badge>
              ) : (
                <Badge tone="gray">{t('acc.usr.otpOff')}</Badge>
              )}
            </li>
            {actions.length > 0 && (
              <li className="flex flex-wrap items-center gap-2">
                <CircleUserRound className="h-4 w-4 text-gray-500" aria-hidden="true" />
                <span className="text-gray-700">{t('acc.usr.pendingSteps')}</span>
                {actions.map((a) => (
                  <Badge key={a} tone="yellow">
                    {requiredActionLabel(a, t)}
                  </Badge>
                ))}
              </li>
            )}
          </ul>
          {manageable && (
            <>
              <div className="flex flex-wrap gap-2">
                <Button variant="secondary" size="sm" disabled={isSelf} onClick={() => onAction('reset-password', user)} aria-label={`${t('acc.usr.resetPassword')} — ${name}`}>
                  <KeyRound className="h-3.5 w-3.5" aria-hidden="true" />
                  {t('acc.usr.resetPassword')}
                </Button>
                <Button variant="secondary" size="sm" disabled={isSelf} onClick={() => onAction('reset-otp', user)} aria-label={`${t('acc.usr.resetOtp')} — ${name}`}>
                  <Smartphone className="h-3.5 w-3.5" aria-hidden="true" />
                  {t('acc.usr.resetOtp')}
                </Button>
                <Button variant="secondary" size="sm" onClick={() => onAction('logout', user)} aria-label={`${t('acc.usr.logout')} — ${name}`}>
                  <LogOut className="h-3.5 w-3.5" aria-hidden="true" />
                  {t('acc.usr.logout')}
                </Button>
              </div>
              {isSelf && <p className="text-xs text-gray-500">{t('acc.usr.selfSecurityHint')}</p>}
            </>
          )}
        </Section>
      </div>
    </Modal>
  )
}
