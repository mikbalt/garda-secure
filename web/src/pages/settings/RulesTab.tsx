import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, Trash2 } from 'lucide-react'
import apiClient, { apiGet } from '../../api/client'
import { RequireSite } from '../../components/RequireSite'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Field, FormError, Input, Select } from '../../components/ui/Form'
import { Card } from '../../components/ui/States'
import { Table, type Column } from '../../components/ui/Table'
import { useErrorToast, useToast } from '../../components/toast/context'
import { useI18n, translateMaybe } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import type { Channel, EventTypes, NotificationRule, Site } from '../../types'

function useEventTypes() {
  return useQuery({
    queryKey: ['event-types'],
    queryFn: () => apiGet<EventTypes>('/notifications/event-types'),
    staleTime: Infinity,
  })
}

function AddRule({ site }: { site: Site }) {
  const { t, lang } = useI18n()
  const qc = useQueryClient()
  const toast = useToast()
  const { data: types } = useEventTypes()
  const [eventType, setEventType] = useState('sos')
  const [channel, setChannel] = useState<Channel>('email')
  const [recipient, setRecipient] = useState('')
  const [formError, setFormError] = useState('')

  const add = useMutation({
    mutationFn: () =>
      apiClient.post('/notifications/rules', { site_id: site.id, event_type: eventType, channel, recipient: recipient.trim() }),
    onSuccess: () => {
      toast.success(t('rule.created'))
      setRecipient('')
      qc.invalidateQueries({ queryKey: ['rules', site.id] })
    },
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!recipient.trim()) return setFormError(`${t('rule.recipient')}: ${t('common.required')}`)
    setFormError('')
    add.mutate()
  }

  return (
    <Card title={t('rule.new')} className="mb-4">
      <form onSubmit={submit} noValidate className="space-y-3">
        <FormError message={formError || (add.error ? errorText(add.error, lang) : null)} />
        <div className="flex flex-wrap items-end gap-3">
          <Field label={t('rule.event')} required>
            <Select value={eventType} onChange={(e) => setEventType(e.target.value)}>
              {(types?.event_types ?? ['sos']).map((ev) => (
                <option key={ev} value={ev}>
                  {translateMaybe(lang, `event.${ev}`, ev)}
                </option>
              ))}
            </Select>
          </Field>
          <Field label={t('rule.channel')} required>
            <Select value={channel} onChange={(e) => setChannel(e.target.value as Channel)}>
              {(types?.channels ?? ['email', 'whatsapp']).map((c) => (
                <option key={c} value={c}>
                  {translateMaybe(lang, `channel.${c}`, c)}
                </option>
              ))}
            </Select>
          </Field>
          <Field
            label={t('rule.recipient')}
            required
            hint={channel === 'email' ? t('rule.recipientHintEmail') : t('rule.recipientHintWa')}
            className="min-w-[16rem] flex-1"
          >
            <Input
              type={channel === 'email' ? 'email' : 'tel'}
              value={recipient}
              onChange={(e) => setRecipient(e.target.value)}
              autoComplete="off"
            />
          </Field>
          <Button type="submit" loading={add.isPending} className="mb-5">
            <Plus className="h-4 w-4" aria-hidden="true" />
            {t('common.add')}
          </Button>
        </div>
      </form>
    </Card>
  )
}

function RulesForSite({ site }: { site: Site }) {
  const { t, lang } = useI18n()
  const qc = useQueryClient()
  const toast = useToast()
  const onError = useErrorToast()
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['rules', site.id],
    queryFn: () => apiGet<NotificationRule[]>('/notifications/rules', { site_id: site.id }),
  })

  const toggle = useMutation({
    mutationFn: (r: NotificationRule) => apiClient.put(`/notifications/rules/${r.id}`, { enabled: !r.enabled }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['rules', site.id] }),
    onError,
  })
  const del = useMutation({
    mutationFn: (id: string) => apiClient.delete(`/notifications/rules/${id}`),
    onSuccess: () => {
      toast.success(t('common.deleted'))
      qc.invalidateQueries({ queryKey: ['rules', site.id] })
    },
    onError,
  })

  const columns: Column<NotificationRule>[] = [
    { key: 'event', header: t('rule.event'), cell: (r) => translateMaybe(lang, `event.${r.event_type}`, r.event_type) },
    { key: 'channel', header: t('rule.channel'), cell: (r) => <Badge tone={r.channel === 'email' ? 'blue' : 'green'}>{translateMaybe(lang, `channel.${r.channel}`, r.channel)}</Badge> },
    { key: 'recipient', header: t('rule.recipient'), cell: (r) => <span className="font-mono text-xs">{r.recipient}</span> },
    {
      key: 'enabled',
      header: t('rule.enabled'),
      cell: (r) => (
        <label className="inline-flex cursor-pointer items-center gap-2">
          <input
            type="checkbox"
            className="h-4 w-4 rounded border-gray-400 text-primary-700 focus:ring-2 focus:ring-primary-600"
            checked={r.enabled}
            disabled={toggle.isPending && toggle.variables?.id === r.id}
            onChange={() => toggle.mutate(r)}
          />
          <span className="sr-only">
            {t('rule.enabled')} — {r.recipient}
          </span>
        </label>
      ),
    },
    {
      key: 'actions',
      header: <span className="sr-only">{t('common.actions')}</span>,
      cell: (r) => (
        <Button
          size="sm"
          variant="ghost"
          className="text-red-700"
          aria-label={`${t('common.delete')} — ${r.recipient}`}
          loading={del.isPending && del.variables === r.id}
          onClick={() => window.confirm(t('common.confirmDelete')) && del.mutate(r.id)}
        >
          <Trash2 className="h-4 w-4" aria-hidden="true" />
        </Button>
      ),
    },
  ]

  return (
    <>
      <p className="mb-3 text-sm text-gray-700">
        {t('common.site')}: <span className="font-medium text-gray-900">{site.name}</span>
      </p>
      <AddRule site={site} />
      <Table columns={columns} rows={data} rowKey={(r) => r.id} isLoading={isLoading} error={error} onRetry={() => refetch()} empty={t('rule.empty')} />
    </>
  )
}

export function RulesTab() {
  return <RequireSite>{(site) => <RulesForSite site={site} />}</RequireSite>
}
