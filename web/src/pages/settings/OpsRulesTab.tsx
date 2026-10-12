import { useId, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { BellOff, Plus } from 'lucide-react'
import apiClient, { apiGet } from '../../api/client'
import { TableSkeleton } from '../../components/LoadingSkeleton'
import { Button } from '../../components/ui/Button'
import { Input } from '../../components/ui/Form'
import { ErrorState, Notice } from '../../components/ui/States'
import { buttonClass } from '../../components/ui/styles'
import { useErrorToast, useToast } from '../../components/toast/context'
import { useI18n, translateMaybe } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import { EVENT_META, channelConfigured, displayRecipient, normaliseRecipient, sortEvents, validRecipient } from './channelMeta'
import { ConfirmDialog, IconTile, SectionHeader } from './kit'
import { ChannelPicker, RecipientChip } from './RulesTab'
import type { Channel, ChannelSettings, EventTypes, OpsRule } from '../../types'

const FALLBACK_OPS_EVENTS = ['ops_backup_failed', 'ops_backup_stale', 'ops_sos_undelivered', 'ops_job_failing']

function InlineAdd({ eventType, channels, onClose }: { eventType: string; channels: Channel[]; onClose: () => void }) {
  const { t, lang } = useI18n()
  const qc = useQueryClient()
  const toast = useToast()
  const [channel, setChannel] = useState<Channel>('email')
  const [recipient, setRecipient] = useState('')
  const [err, setErr] = useState('')
  const inputId = useId()
  const hintId = useId()
  const add = useMutation({
    mutationFn: (b: { event_type: string; channel: Channel; recipient: string }) => apiClient.post('/notifications/ops-rules', b),
    onSuccess: () => {
      toast.success(t('adm.rule.added'))
      qc.invalidateQueries({ queryKey: ['ops-rules'] })
      onClose()
    },
  })
  const hint = channel === 'email' ? t('adm.rule.hintEmail') : t('adm.rule.hintWa')

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const r = normaliseRecipient(channel, recipient)
    if (!validRecipient(channel, r)) return setErr(hint)
    setErr('')
    add.mutate({ event_type: eventType, channel, recipient: r })
  }

  const message = err || (add.error ? errorText(add.error, lang) : '')
  return (
    <form onSubmit={submit} noValidate className="mt-3 rounded-xl border border-gray-200 bg-gray-50/70 p-3">
      <div className="flex flex-wrap items-start gap-2">
        <ChannelPicker value={channel} onChange={setChannel} channels={channels} />
        <div className="min-w-[12rem] flex-1">
          <label htmlFor={inputId} className="sr-only">
            {t('rule.recipient')}
          </label>
          <Input
            id={inputId}
            autoFocus
            type={channel === 'email' ? 'email' : 'tel'}
            inputMode={channel === 'email' ? 'email' : 'tel'}
            placeholder={channel === 'email' ? t('plat.phEmail') : t('plat.phWa')}
            value={recipient}
            onChange={(e) => setRecipient(e.target.value)}
            aria-invalid={message ? true : undefined}
            aria-describedby={hintId}
            className="h-9 !py-1.5"
            autoComplete="off"
          />
        </div>
        <div className="flex gap-2">
          <Button type="submit" loading={add.isPending}>
            {t('common.add')}
          </Button>
          <Button variant="ghost" onClick={onClose}>
            {t('common.cancel')}
          </Button>
        </div>
      </div>
      <p id={hintId} className={`mt-2 text-xs ${message ? 'font-medium text-red-700' : 'text-gray-600'}`} role={message ? 'alert' : undefined}>
        {message || hint}
      </p>
    </form>
  )
}

/**
 * Install-wide operations alerts (D-027, platform-admin only): who is told
 * when a backup fails or is stale, an SOS could not be delivered, or a
 * scheduled job keeps failing. Same recipients model as the per-site rules,
 * without a site.
 */
export function OpsRulesTab() {
  const { t, lang } = useI18n()
  const qc = useQueryClient()
  const toast = useToast()
  const onError = useErrorToast()
  const [adding, setAdding] = useState<string | null>(null)
  const [removing, setRemoving] = useState<OpsRule | null>(null)

  const types = useQuery({
    queryKey: ['ops-event-types'],
    queryFn: () => apiGet<EventTypes>('/notifications/ops-event-types'),
    staleTime: Infinity,
  })
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['ops-rules'],
    queryFn: () => apiGet<OpsRule[]>('/notifications/ops-rules'),
  })
  const channelsQ = useQuery({ queryKey: ['channels'], queryFn: () => apiGet<ChannelSettings>('/settings/channels') })

  const toggle = useMutation({
    mutationFn: (r: OpsRule) => apiClient.put(`/notifications/ops-rules/${r.id}`, { enabled: !r.enabled }),
    onSuccess: (_d, r) => {
      toast.success(r.enabled ? t('adm.rule.paused') : t('adm.rule.resumed'))
      qc.invalidateQueries({ queryKey: ['ops-rules'] })
    },
    onError,
  })
  const del = useMutation({
    mutationFn: (id: string) => apiClient.delete(`/notifications/ops-rules/${id}`),
    onSuccess: () => {
      toast.success(t('common.deleted'))
      qc.invalidateQueries({ queryKey: ['ops-rules'] })
      setRemoving(null)
    },
    onError,
  })

  const rules = data ?? []
  const events = sortEvents(Array.from(new Set([...(types.data?.event_types ?? FALLBACK_OPS_EVENTS), ...rules.map((r) => r.event_type)])))
  const channels: Channel[] = types.data?.channels?.length ? types.data.channels : ['email', 'whatsapp']
  const unconfigured = channelsQ.data ? channels.filter((c) => rules.some((r) => r.channel === c && r.enabled) && !channelConfigured(channelsQ.data, c)) : []
  const title = (ev: string) => (EVENT_META[ev] ? t(EVENT_META[ev].title) : translateMaybe(lang, `event.${ev}`, ev))

  return (
    <>
      <SectionHeader title={t('plat.opsRules')} description={t('plat.opsRulesIntro')} />
      <Notice>{t('plat.opsRulesNote')}</Notice>
      {unconfigured.length > 0 && (
        <Notice tone="warn">
          <span className="font-medium">{t('adm.rule.channelMissing', { channels: unconfigured.map((c) => translateMaybe(lang, `channel.${c}`, c)).join(' & ') })}</span>{' '}
          <Link to="/settings?tab=channels" className="font-semibold underline underline-offset-2 hover:text-amber-950">
            {t('adm.rule.channelMissingCta')}
          </Link>
        </Notice>
      )}
      {isLoading ? (
        <TableSkeleton />
      ) : error ? (
        <ErrorState error={error} onRetry={() => refetch()} />
      ) : (
        <ul className="space-y-3">
          {events.map((ev) => {
            const meta = EVENT_META[ev]
            const list = rules.filter((r) => r.event_type === ev).sort((a, b) => a.channel.localeCompare(b.channel) || a.recipient.localeCompare(b.recipient))
            const active = list.filter((r) => r.enabled).length
            return (
              <li key={ev} className="rounded-2xl border border-gray-200/80 bg-white p-4 shadow-card sm:p-5">
                <div className="flex gap-3.5">
                  {meta && <IconTile icon={meta.icon} tone={meta.tone} />}
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
                      <h3 className="text-[15px] font-semibold text-gray-900">{title(ev)}</h3>
                      <span className="text-xs font-medium text-gray-600">{active ? t('adm.rule.recipients', { n: active }) : t('adm.rule.noRecipients')}</span>
                    </div>
                    {meta && <p className="mt-0.5 text-sm text-gray-600">{t(meta.desc)}</p>}
                    <ul className="mt-3 flex flex-wrap items-center gap-2" aria-label={t('adm.rule.recipientsOf', { event: title(ev) })}>
                      {list.map((r) => (
                        <RecipientChip
                          key={r.id}
                          rule={r}
                          busy={toggle.isPending && toggle.variables?.id === r.id}
                          onToggle={() => toggle.mutate(r)}
                          onRemove={() => setRemoving(r)}
                        />
                      ))}
                      {adding !== ev && (
                        <li>
                          <button
                            type="button"
                            onClick={() => setAdding(ev)}
                            className={buttonClass('ghost', 'sm', 'rounded-full border border-dashed border-gray-300 text-gray-700 hover:border-primary-300 hover:text-primary-700')}
                            aria-label={`${t('adm.rule.addShort')} — ${title(ev)}`}
                          >
                            <Plus className="h-3.5 w-3.5" aria-hidden="true" />
                            {t('adm.rule.addShort')}
                          </button>
                        </li>
                      )}
                    </ul>
                    {list.length === 0 && adding !== ev && (
                      <p className="mt-2 inline-flex items-center gap-1.5 text-xs font-medium text-amber-800">
                        <BellOff className="h-3.5 w-3.5" aria-hidden="true" />
                        {t('plat.opsNobody')}
                      </p>
                    )}
                    {adding === ev && <InlineAdd eventType={ev} channels={channels} onClose={() => setAdding(null)} />}
                  </div>
                </div>
              </li>
            )
          })}
        </ul>
      )}
      <ConfirmDialog
        open={!!removing}
        title={t('adm.rule.removeTitle')}
        message={removing ? t('adm.rule.removeBody', { to: displayRecipient(removing.channel, removing.recipient), event: title(removing.event_type) }) : ''}
        confirmLabel={t('common.delete')}
        loading={del.isPending}
        onConfirm={() => removing && del.mutate(removing.id)}
        onClose={() => setRemoving(null)}
      />
    </>
  )
}
