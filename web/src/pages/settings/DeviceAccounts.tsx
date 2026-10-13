import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound, Plus, Tablet, UserRoundCog } from 'lucide-react'
import apiClient, { apiGet } from '../../api/client'
import { SecretOnce } from '../../components/SecretOnce'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Field, FormError, Input } from '../../components/ui/Form'
import { Modal } from '../../components/ui/Modal'
import { Card, ErrorState } from '../../components/ui/States'
import { useErrorToast, useToast } from '../../components/toast/context'
import { useAuth } from '../../hooks/useAuth'
import { useNewParam } from '../../hooks/useNewParam'
import { useI18n } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import type { DeviceAccount, DeviceAccountSecret } from '../../types'
import { ConfirmDialog, Switch } from './kit'
import { relativeTime } from './format'
import { deviceUsername } from './accountUtil'

/** Where and how to sign in on the tablet (server address + organisation code from the running app). */
export function TabletSignInHint({ username }: { username: string }) {
  const { t } = useI18n()
  const { user } = useAuth()
  const host = typeof window !== 'undefined' ? window.location.host : ''
  const item = 'grid grid-cols-[minmax(0,10rem)_minmax(0,1fr)] gap-3 px-3.5 py-2 text-sm'
  return (
    <div className="rounded-xl border border-primary-100 bg-primary-50/60 p-3.5">
      <p className="text-sm font-semibold text-primary-900">{t('acc.dev.howSignIn')}</p>
      <ol className="mt-1.5 list-decimal space-y-1 pl-5 text-sm text-primary-900">
        <li>{t('acc.dev.howStep1')}</li>
        <li>{t('acc.dev.howStep2')}</li>
        <li>{t('acc.dev.howStep3')}</li>
      </ol>
      <dl className="mt-3 divide-y divide-primary-100 overflow-hidden rounded-lg bg-white ring-1 ring-inset ring-primary-100">
        <div className={item}>
          <dt className="text-gray-600">{t('acc.dev.serverAddress')}</dt>
          <dd className="break-all font-mono text-gray-900">{host}</dd>
        </div>
        <div className={item}>
          <dt className="text-gray-600">{t('acc.dev.orgCode')}</dt>
          <dd className="break-all font-mono text-gray-900">{user.realm}</dd>
        </div>
        <div className={item}>
          <dt className="text-gray-600">{t('acc.dev.username')}</dt>
          <dd className="break-all font-mono text-gray-900">{username}</dd>
        </div>
      </dl>
    </div>
  )
}

function SecretModal({ title, secret, onClose }: { title: string; secret: DeviceAccountSecret; onClose: () => void }) {
  const { t } = useI18n()
  return (
    <Modal open onClose={onClose} size="lg" title={title} footer={<Button onClick={onClose}>{t('acc.secret.done')}</Button>}>
      <SecretOnce label={t('acc.dev.password')} value={secret.password}>
        <TabletSignInHint username={secret.account.username} />
      </SecretOnce>
    </Modal>
  )
}

function CreateModal({ onClose }: { onClose: () => void }) {
  const { t, lang } = useI18n()
  const qc = useQueryClient()
  const [name, setName] = useState('')
  const [error, setError] = useState('')
  const [secret, setSecret] = useState<DeviceAccountSecret | null>(null)
  const create = useMutation({
    mutationFn: () => apiClient.post<DeviceAccountSecret>('/device-accounts', { name: name.trim() }).then((r) => r.data),
    onSuccess: (res) => {
      qc.invalidateQueries({ queryKey: ['device-accounts'] })
      setSecret(res)
    },
  })
  if (secret) return <SecretModal title={t('acc.dev.createdTitle', { name: secret.account.name })} secret={secret} onClose={onClose} />
  const preview = deviceUsername(name)
  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!preview) return setError(t('acc.dev.nameRequired'))
    setError('')
    create.mutate()
  }
  return (
    <Modal
      open
      onClose={onClose}
      title={t('acc.dev.createTitle')}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="device-account-form" loading={create.isPending}>
            <Plus className="h-4 w-4" aria-hidden="true" />
            {t('acc.dev.create')}
          </Button>
        </>
      }
    >
      <form id="device-account-form" onSubmit={submit} noValidate className="space-y-4">
        <p className="text-sm text-gray-600">{t('acc.dev.createIntro')}</p>
        <FormError message={create.error ? errorText(create.error, lang) : null} />
        <Field label={t('acc.dev.name')} required error={error} hint={preview ? t('acc.dev.usernamePreview', { username: preview }) : t('acc.dev.nameHint')}>
          <Input value={name} onChange={(e) => setName(e.target.value)} maxLength={60} autoComplete="off" />
        </Field>
      </form>
    </Modal>
  )
}

/**
 * Settings → Tablets: the Keycloak accounts tablets sign in with (D-041).
 * Create one per tablet, hand over the password shown once, reset it when a
 * tablet is replaced, disable it when a tablet is lost.
 */
export function DeviceAccounts() {
  const { t, lang } = useI18n()
  const qc = useQueryClient()
  const toast = useToast()
  const onError = useErrorToast()
  const [creating, setCreating] = useState(false)
  const [resetting, setResetting] = useState<DeviceAccount | null>(null)
  const [disabling, setDisabling] = useState<DeviceAccount | null>(null)
  const [secret, setSecret] = useState<DeviceAccountSecret | null>(null)

  useNewParam(() => setCreating(true))

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['device-accounts'],
    queryFn: () => apiGet<DeviceAccount[]>('/device-accounts'),
  })

  const reset = useMutation({
    mutationFn: (a: DeviceAccount) => apiClient.post<DeviceAccountSecret>(`/device-accounts/${a.id}/reset-password`).then((r) => r.data),
    onSuccess: (res) => {
      setResetting(null)
      setSecret(res)
    },
    onError,
  })
  const setEnabled = useMutation({
    mutationFn: ({ a, enabled }: { a: DeviceAccount; enabled: boolean }) => apiClient.put(`/device-accounts/${a.id}`, { enabled }),
    onSuccess: (_r, { a, enabled }) => {
      toast.success(enabled ? t('acc.dev.enabledToast', { name: a.name }) : t('acc.dev.disabledToast', { name: a.name }))
      qc.invalidateQueries({ queryKey: ['device-accounts'] })
      setDisabling(null)
    },
    onError,
  })

  const rows = [...(data ?? [])].sort((a, b) => Number(b.enabled) - Number(a.enabled) || a.name.localeCompare(b.name))
  const now = Date.now()

  return (
    <Card
      title={t('acc.dev.title', { n: rows.length })}
      description={t('acc.dev.desc')}
      flush
      actions={
        <Button size="sm" onClick={() => setCreating(true)}>
          <Plus className="h-3.5 w-3.5" aria-hidden="true" />
          {t('acc.dev.add')}
        </Button>
      }
    >
      {isLoading ? (
        <p className="px-5 py-6 text-sm text-gray-500">{t('app.loading')}</p>
      ) : error ? (
        <div className="p-4">
          <ErrorState error={error} onRetry={() => refetch()} />
        </div>
      ) : rows.length === 0 ? (
        <div className="flex flex-col items-start gap-3 px-5 py-6 text-sm text-gray-600 sm:flex-row sm:items-center">
          <UserRoundCog className="h-5 w-5 shrink-0 text-gray-500" aria-hidden="true" />
          <p className="min-w-0 flex-1">{t('acc.dev.empty')}</p>
          <Button size="sm" variant="secondary" onClick={() => setCreating(true)}>
            {t('acc.dev.add')}
          </Button>
        </div>
      ) : (
        <ul className="divide-y divide-gray-100">
          {rows.map((a) => (
            <li key={a.id} className="flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3 sm:px-5">
              <div className={`min-w-0 flex-1 ${a.enabled ? '' : 'opacity-70'}`}>
                <p className="flex flex-wrap items-center gap-2 text-sm font-medium text-gray-900">
                  <span className="truncate">{a.name}</span>
                  {!a.enabled && <Badge tone="red" dot>{t('acc.dev.disabled')}</Badge>}
                </p>
                <p className="flex flex-wrap items-center gap-x-3 gap-y-0.5 text-xs text-gray-500">
                  <span className="font-mono text-gray-600">{a.username}</span>
                  {a.device ? (
                    <a
                      href={`#device-${a.device.id}`}
                      className="inline-flex items-center gap-1 text-primary-700 underline-offset-2 hover:underline focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
                    >
                      <Tablet className="h-3 w-3" aria-hidden="true" />
                      {t('acc.dev.usedBy', { name: a.device.name, status: t(`acc.dev.status.${a.device.status}`) })}
                    </a>
                  ) : (
                    <span>{t('acc.dev.notUsed')}</span>
                  )}
                  <span>{t('acc.dev.created', { when: relativeTime(a.created_at, lang, now) })}</span>
                </p>
              </div>
              <div className="flex items-center gap-2">
                <Switch
                  checked={a.enabled}
                  label={`${t('acc.dev.accountEnabled')} — ${a.name}`}
                  disabled={setEnabled.isPending}
                  onChange={(on) => (on ? setEnabled.mutate({ a, enabled: true }) : setDisabling(a))}
                />
                <Button size="sm" variant="secondary" onClick={() => setResetting(a)} aria-label={`${t('acc.dev.resetPassword')} — ${a.name}`}>
                  <KeyRound className="h-3.5 w-3.5" aria-hidden="true" />
                  <span className="hidden sm:inline">{t('acc.dev.resetPassword')}</span>
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}

      {creating && <CreateModal onClose={() => setCreating(false)} />}
      {secret && <SecretModal title={t('acc.dev.resetDoneTitle', { name: secret.account.name })} secret={secret} onClose={() => setSecret(null)} />}
      <ConfirmDialog
        open={!!resetting}
        title={t('acc.dev.resetTitle', { name: resetting?.name ?? '' })}
        message={t('acc.dev.resetBody')}
        confirmLabel={t('acc.dev.resetPassword')}
        loading={reset.isPending}
        onConfirm={() => resetting && reset.mutate(resetting)}
        onClose={() => setResetting(null)}
      />
      <ConfirmDialog
        open={!!disabling}
        title={t('acc.dev.disableTitle', { name: disabling?.name ?? '' })}
        message={t('acc.dev.disableBody')}
        confirmLabel={t('acc.dev.disable')}
        loading={setEnabled.isPending}
        onConfirm={() => disabling && setEnabled.mutate({ a: disabling, enabled: false })}
        onClose={() => setDisabling(null)}
      />
    </Card>
  )
}
