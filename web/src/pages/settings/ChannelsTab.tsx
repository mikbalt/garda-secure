import { useRef, useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CheckCircle2, CircleDashed, Info, Send } from 'lucide-react'
import apiClient, { apiErrorCode, apiGet } from '../../api/client'
import { TableSkeleton } from '../../components/LoadingSkeleton'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Field, FormError, Input } from '../../components/ui/Form'
import { Modal } from '../../components/ui/Modal'
import { ErrorState, Notice } from '../../components/ui/States'
import { useToast } from '../../components/toast/context'
import { useI18n, type Key } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import { CHANNEL_ICON, channelConfigured, normaliseRecipient, validRecipient } from './channelMeta'
import { IconTile, SectionHeader } from './kit'
import type { Channel, ChannelSettings } from '../../types'

const EMPTY: ChannelSettings = {
  smtp: { host: '', port: '', user: '', password: '', from: '' },
  whatsapp: { api_url: '', token: '', phone_number_id: '', template_name: '', template_lang: '' },
}

interface FieldDef {
  key: string
  label: Key
  hint?: Key
  placeholder?: string
  /** Translated placeholder (language-specific examples). */
  placeholderKey?: Key
  secret?: boolean
  type?: string
  wide?: boolean
}

/** The masked value the API returns for a stored secret. */
const MASK = '********'
/** Fields that decide where a secret is sent: changing them requires re-entering it (D-032). */
const DESTINATION_FIELDS: Record<Channel, string[]> = { email: ['host', 'port', 'user'], whatsapp: ['api_url', 'phone_number_id'] }
const SECRET_FIELD: Record<Channel, string> = { email: 'password', whatsapp: 'token' }

const SMTP_FIELDS: FieldDef[] = [
  { key: 'host', label: 'ch.host', hint: 'adm.ch.hint.host', placeholder: 'smtp.gmail.com' },
  { key: 'port', label: 'ch.port', hint: 'adm.ch.hint.port', placeholder: '587', type: 'number' },
  { key: 'user', label: 'ch.user', hint: 'adm.ch.hint.user' },
  { key: 'password', label: 'ch.password', hint: 'adm.ch.hint.secret', secret: true },
  { key: 'from', label: 'ch.from', hint: 'adm.ch.hint.from', placeholderKey: 'plat.phFrom', type: 'email', wide: true },
]
const WA_FIELDS: FieldDef[] = [
  { key: 'api_url', label: 'ch.apiUrl', hint: 'adm.ch.hint.apiUrl', placeholder: 'https://graph.facebook.com/v18.0', type: 'url', wide: true },
  { key: 'token', label: 'ch.token', hint: 'adm.ch.hint.secret', secret: true, wide: true },
  { key: 'phone_number_id', label: 'ch.phoneNumberId', hint: 'adm.ch.hint.phoneId' },
  { key: 'template_lang', label: 'ch.templateLang', hint: 'adm.ch.hint.lang', placeholder: 'id' },
  { key: 'template_name', label: 'ch.templateName', hint: 'adm.ch.hint.template', placeholder: 'securepatrol_alert', wide: true },
]

function TestModal({ channel, onClose }: { channel: Channel; onClose: () => void }) {
  const { t, lang } = useI18n()
  const toast = useToast()
  const [recipient, setRecipient] = useState('')
  const [err, setErr] = useState('')
  const send = useMutation({
    mutationFn: (r: string) => apiClient.post('/settings/channels/test', { channel, recipient: r }),
    onSuccess: () => {
      toast.success(t('ch.testSent'))
      onClose()
    },
  })
  const submit = (e: FormEvent) => {
    e.preventDefault()
    const r = normaliseRecipient(channel, recipient)
    if (!validRecipient(channel, r)) return setErr(channel === 'email' ? t('adm.rule.hintEmail') : t('adm.rule.hintWa'))
    setErr('')
    send.mutate(r)
  }
  return (
    <Modal
      open
      onClose={onClose}
      title={channel === 'email' ? t('adm.ch.testEmail') : t('adm.ch.testWa')}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="channel-test-form" loading={send.isPending}>
            <Send className="h-4 w-4" aria-hidden="true" />
            {t('ch.test')}
          </Button>
        </>
      }
    >
      <form id="channel-test-form" onSubmit={submit} noValidate className="space-y-4">
        <p className="text-sm text-gray-600">{t('adm.ch.testIntro')}</p>
        <FormError message={send.error ? errorText(send.error, lang) : null} />
        <Field label={t('adm.rule.to')} required hint={channel === 'email' ? t('adm.rule.hintEmail') : t('adm.rule.hintWa')} error={err || undefined}>
          <Input
            value={recipient}
            onChange={(e) => setRecipient(e.target.value)}
            type={channel === 'email' ? 'email' : 'tel'}
            inputMode={channel === 'email' ? 'email' : 'tel'}
            placeholder={channel === 'email' ? t('plat.phEmail') : t('plat.phWa')}
          />
        </Field>
      </form>
    </Modal>
  )
}

type Part<K extends Channel> = K extends 'email' ? ChannelSettings['smtp'] : ChannelSettings['whatsapp']

function ChannelCard<K extends Channel>({ channel, initial, all }: { channel: K; initial: Part<K>; all: ChannelSettings }) {
  const { t, lang } = useI18n()
  const qc = useQueryClient()
  const toast = useToast()
  const [values, setValues] = useState<Record<string, string>>(() => ({ ...(initial as unknown as Record<string, string>) }))
  const [testing, setTesting] = useState(false)
  const [secretErr, setSecretErr] = useState('')
  const secretRef = useRef<HTMLInputElement>(null)
  const part = channel === 'email' ? 'smtp' : 'whatsapp'
  const fields = channel === 'email' ? SMTP_FIELDS : WA_FIELDS
  const configured = channelConfigured(all, channel)
  const init = initial as unknown as Record<string, string>
  const dirty = fields.some((f) => (values[f.key] ?? '') !== (init[f.key] ?? ''))
  const secretKey = SECRET_FIELD[channel]
  const destinationChanged = DESTINATION_FIELDS[channel].some((k) => (values[k] ?? '') !== (init[k] ?? ''))
  // A stored secret may only be kept while the destination is unchanged.
  const needsSecret = destinationChanged && init[secretKey] === MASK && (values[secretKey] ?? '') === MASK
  const Icon = CHANNEL_ICON[channel]

  const save = useMutation({
    mutationFn: async () => {
      const current = qc.getQueryData<ChannelSettings>(['channels']) ?? all
      return (await apiClient.put<ChannelSettings>('/settings/channels', { ...current, [part]: values })).data
    },
    onSuccess: (saved) => {
      toast.success(t('common.saved'))
      qc.setQueryData(['channels'], saved)
      setValues({ ...(saved[part] as unknown as Record<string, string>) })
      setSecretErr('')
    },
    onError: (err) => {
      if (apiErrorCode(err) === 'secret_required') {
        setSecretErr(t('plat.secretReenter'))
        secretRef.current?.focus()
      }
    },
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (needsSecret) {
      setSecretErr(t('plat.secretReenter'))
      secretRef.current?.focus()
      return
    }
    setSecretErr('')
    save.mutate()
  }

  const saveError = save.error && apiErrorCode(save.error) !== 'secret_required' ? errorText(save.error, lang) : null

  const formId = `channel-${channel}`
  return (
    <section className="rounded-2xl border border-gray-200/80 bg-white shadow-card" aria-labelledby={`${formId}-title`}>
      <div className="flex flex-wrap items-start gap-3.5 border-b border-gray-100 px-5 py-4">
        <IconTile icon={Icon} tone={channel === 'email' ? 'blue' : 'green'} />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h3 id={`${formId}-title`} className="text-[15px] font-semibold text-gray-900">
              {channel === 'email' ? t('adm.ch.email') : t('adm.ch.wa')}
            </h3>
            {configured ? (
              <Badge tone="green">
                <CheckCircle2 className="h-3.5 w-3.5" aria-hidden="true" />
                {t('adm.ch.configured')}
              </Badge>
            ) : (
              <Badge tone="gray">
                <CircleDashed className="h-3.5 w-3.5" aria-hidden="true" />
                {t('adm.ch.notConfigured')}
              </Badge>
            )}
          </div>
          <p className="mt-0.5 text-sm text-gray-600">{channel === 'email' ? t('adm.ch.emailDesc') : t('adm.ch.waDesc')}</p>
        </div>
      </div>

      <form id={formId} onSubmit={submit} noValidate className="px-5 py-5">
        {channel === 'whatsapp' && (
          <div className="mb-5 flex gap-2.5 rounded-xl border border-emerald-100 bg-emerald-50/60 px-3.5 py-3 text-sm text-emerald-950">
            <Info className="mt-0.5 h-4 w-4 shrink-0 text-emerald-600" aria-hidden="true" />
            <div>
              <p className="font-medium">{t('adm.ch.waTemplateTitle')}</p>
              <p className="mt-0.5 text-emerald-900/80">{t('adm.ch.waTemplateBody')}</p>
            </div>
          </div>
        )}
        <FormError message={saveError} />
        {needsSecret && !secretErr && <Notice tone="warn">{t('plat.secretHint')}</Notice>}
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          {fields.map((f) => (
            <Field
              key={f.key}
              label={t(f.label)}
              hint={f.hint ? t(f.hint) : undefined}
              error={f.key === secretKey && secretErr ? secretErr : undefined}
              required={f.key === secretKey && needsSecret}
              className={f.wide ? 'sm:col-span-2' : ''}
            >
              <Input
                ref={f.key === secretKey ? secretRef : undefined}
                type={f.secret ? 'password' : f.type ?? 'text'}
                autoComplete={f.secret ? 'new-password' : 'off'}
                placeholder={f.placeholderKey ? t(f.placeholderKey) : f.placeholder}
                value={values[f.key] ?? ''}
                onFocus={f.key === secretKey && values[f.key] === MASK && needsSecret ? (e) => e.target.select() : undefined}
                onChange={(e) => {
                  setValues((v) => ({ ...v, [f.key]: e.target.value }))
                  if (f.key === secretKey) setSecretErr('')
                }}
              />
            </Field>
          ))}
        </div>
      </form>

      <div className="flex flex-wrap items-center justify-end gap-2 rounded-b-2xl border-t border-gray-100 bg-gray-50/70 px-5 py-3">
        {dirty && <span className="mr-auto text-xs font-medium text-amber-800">{t('adm.ch.unsaved')}</span>}
        <Button variant="secondary" onClick={() => setTesting(true)} disabled={!configured || dirty} title={!configured ? t('adm.ch.testNeedsConfig') : dirty ? t('adm.ch.testNeedsSave') : undefined}>
          <Send className="h-4 w-4" aria-hidden="true" />
          {t('ch.test')}
        </Button>
        <Button type="submit" form={formId} loading={save.isPending} disabled={!dirty}>
          {t('common.save')}
        </Button>
      </div>
      {testing && <TestModal channel={channel} onClose={() => setTesting(false)} />}
    </section>
  )
}

/** SMTP + WhatsApp channel settings (install-wide, D-005). Secrets come back as ********. */
export function ChannelsTab() {
  const { t } = useI18n()
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['channels'],
    queryFn: () => apiGet<ChannelSettings>('/settings/channels'),
  })
  const all: ChannelSettings | undefined = data ? { smtp: { ...EMPTY.smtp, ...data.smtp }, whatsapp: { ...EMPTY.whatsapp, ...data.whatsapp } } : undefined

  return (
    <>
      <SectionHeader title={t('adm.set.channels')} description={t('adm.ch.intro')} />
      <Notice>
        {t('ch.installWide')} {t('adm.ch.secretNote')}
      </Notice>
      {isLoading ? (
        <TableSkeleton />
      ) : error || !all ? (
        <ErrorState error={error} onRetry={() => refetch()} />
      ) : (
        <div className="space-y-6">
          <ChannelCard channel="whatsapp" initial={all.whatsapp} all={all} />
          <ChannelCard channel="email" initial={all.smtp} all={all} />
        </div>
      )}
    </>
  )
}
