import { useEffect, useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Send } from 'lucide-react'
import apiClient, { apiGet } from '../../api/client'
import { TableSkeleton } from '../../components/LoadingSkeleton'
import { Button } from '../../components/ui/Button'
import { Field, FormError, Input, Select } from '../../components/ui/Form'
import { Modal } from '../../components/ui/Modal'
import { Card, ErrorState, Notice } from '../../components/ui/States'
import { useToast } from '../../components/toast/context'
import { useI18n, type Key } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import type { Channel, ChannelSettings } from '../../types'

const EMPTY: ChannelSettings = {
  smtp: { host: '', port: '', user: '', password: '', from: '' },
  whatsapp: { api_url: '', token: '', phone_number_id: '', template_name: '', template_lang: '' },
}

type SmtpKey = keyof ChannelSettings['smtp']
type WaKey = keyof ChannelSettings['whatsapp']

const SMTP_FIELDS: { key: SmtpKey; label: Key; secret?: boolean; type?: string }[] = [
  { key: 'host', label: 'ch.host' },
  { key: 'port', label: 'ch.port', type: 'number' },
  { key: 'user', label: 'ch.user' },
  { key: 'password', label: 'ch.password', secret: true },
  { key: 'from', label: 'ch.from', type: 'email' },
]
const WA_FIELDS: { key: WaKey; label: Key; secret?: boolean; type?: string }[] = [
  { key: 'api_url', label: 'ch.apiUrl', type: 'url' },
  { key: 'token', label: 'ch.token', secret: true },
  { key: 'phone_number_id', label: 'ch.phoneNumberId' },
  { key: 'template_name', label: 'ch.templateName' },
  { key: 'template_lang', label: 'ch.templateLang' },
]

function TestModal({ onClose }: { onClose: () => void }) {
  const { t, lang } = useI18n()
  const toast = useToast()
  const [channel, setChannel] = useState<Channel>('email')
  const [recipient, setRecipient] = useState('')
  const send = useMutation({
    mutationFn: () => apiClient.post('/settings/channels/test', { channel, recipient: recipient.trim() }),
    onSuccess: () => {
      toast.success(t('ch.testSent'))
      onClose()
    },
  })
  return (
    <Modal
      open
      onClose={onClose}
      title={t('ch.testTitle')}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button onClick={() => send.mutate()} loading={send.isPending} disabled={!recipient.trim()}>
            <Send className="h-4 w-4" aria-hidden="true" />
            {t('ch.test')}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <FormError message={send.error ? errorText(send.error, lang) : null} />
        <Field label={t('rule.channel')} required>
          <Select value={channel} onChange={(e) => setChannel(e.target.value as Channel)}>
            <option value="email">{t('channel.email')}</option>
            <option value="whatsapp">{t('channel.whatsapp')}</option>
          </Select>
        </Field>
        <Field label={t('rule.recipient')} required hint={channel === 'email' ? t('rule.recipientHintEmail') : t('rule.recipientHintWa')}>
          <Input value={recipient} onChange={(e) => setRecipient(e.target.value)} type={channel === 'email' ? 'email' : 'tel'} />
        </Field>
      </div>
    </Modal>
  )
}

/** SMTP + WhatsApp channel settings (install-wide, D-005). Secrets come back as ********. */
export function ChannelsTab() {
  const { t, lang } = useI18n()
  const qc = useQueryClient()
  const toast = useToast()
  const [form, setForm] = useState<ChannelSettings>(EMPTY)
  const [testing, setTesting] = useState(false)

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['channels'],
    queryFn: () => apiGet<ChannelSettings>('/settings/channels'),
  })
  useEffect(() => {
    if (data) setForm({ smtp: { ...EMPTY.smtp, ...data.smtp }, whatsapp: { ...EMPTY.whatsapp, ...data.whatsapp } })
  }, [data])

  const save = useMutation({
    mutationFn: async () => (await apiClient.put<ChannelSettings>('/settings/channels', form)).data,
    onSuccess: (saved) => {
      toast.success(t('common.saved'))
      qc.setQueryData(['channels'], saved)
    },
  })

  if (isLoading) return <TableSkeleton />
  if (error) return <ErrorState error={error} onRetry={() => refetch()} />

  const submit = (e: FormEvent) => {
    e.preventDefault()
    save.mutate()
  }

  return (
    <form onSubmit={submit} noValidate>
      <Notice>
        {t('ch.installWide')} {t('ch.secretHint')}
      </Notice>
      <FormError message={save.error ? errorText(save.error, lang) : null} />
      <div className="mt-2 grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card title={t('ch.smtp')}>
          <div className="space-y-3">
            {SMTP_FIELDS.map((f) => (
              <Field key={f.key} label={t(f.label)}>
                <Input
                  type={f.secret ? 'password' : f.type ?? 'text'}
                  autoComplete={f.secret ? 'new-password' : 'off'}
                  value={form.smtp[f.key]}
                  onChange={(e) => setForm((s) => ({ ...s, smtp: { ...s.smtp, [f.key]: e.target.value } }))}
                />
              </Field>
            ))}
          </div>
        </Card>
        <Card title={t('ch.whatsapp')}>
          <div className="space-y-3">
            {WA_FIELDS.map((f) => (
              <Field key={f.key} label={t(f.label)}>
                <Input
                  type={f.secret ? 'password' : f.type ?? 'text'}
                  autoComplete={f.secret ? 'new-password' : 'off'}
                  value={form.whatsapp[f.key]}
                  onChange={(e) => setForm((s) => ({ ...s, whatsapp: { ...s.whatsapp, [f.key]: e.target.value } }))}
                />
              </Field>
            ))}
          </div>
        </Card>
      </div>
      <div className="mt-4 flex justify-end gap-2">
        <Button variant="secondary" onClick={() => setTesting(true)}>
          <Send className="h-4 w-4" aria-hidden="true" />
          {t('ch.test')}
        </Button>
        <Button type="submit" loading={save.isPending}>
          {t('common.save')}
        </Button>
      </div>
      {testing && <TestModal onClose={() => setTesting(false)} />}
    </form>
  )
}
