import { useId, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { BellOff, BellRing, Building2, CheckCircle2, Plus, X, XCircle } from 'lucide-react'
import apiClient, { apiGet } from '../../api/client'
import { RequireSite } from '../../components/RequireSite'
import { TableSkeleton } from '../../components/LoadingSkeleton'
import { Button } from '../../components/ui/Button'
import { Field, FormError, Input, Select } from '../../components/ui/Form'
import { Modal } from '../../components/ui/Modal'
import { EmptyState, ErrorState, Notice } from '../../components/ui/States'
import { buttonClass } from '../../components/ui/styles'
import { useErrorToast, useToast } from '../../components/toast/context'
import { useSite } from '../../context/site'
import { useAuth } from '../../hooks/useAuth'
import { useNewParam } from '../../hooks/useNewParam'
import { useI18n, translateMaybe } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import {
  EVENT_META,
  channelIcon,
  channelTone,
  displayRecipient,
  normaliseRecipient,
  sortEvents,
  validRecipient,
} from './channelMeta'
import { ConfirmDialog, IconTile, SectionHeader, Switch } from './kit'
import type { Channel, ChannelInfo, EventTypes, NotificationRule, Site } from '../../types'

const FALLBACK_EVENTS = ['sos', 'incident_high', 'incident_filed', 'absent', 'late_clock_in', 'patrol_not_started', 'sla_missed', 'defect_opened', 'monthly_recap']

/** Registered delivery channels and whether they are set up (GET /notifications/channels, D-047). */
function useChannels() {
  return useQuery({
    queryKey: ['notification-channels'],
    queryFn: () => apiGet<ChannelInfo[]>('/notifications/channels'),
    staleTime: 60_000,
  })
}

function useEventTypes() {
  return useQuery({
    queryKey: ['event-types'],
    queryFn: () => apiGet<EventTypes>('/notifications/event-types'),
    staleTime: Infinity,
  })
}

function useAddRule(site: Site, onDone: () => void) {
  const { t } = useI18n()
  const qc = useQueryClient()
  const toast = useToast()
  return useMutation({
    mutationFn: (b: { event_type: string; channel: string; recipient: string }) => apiClient.post('/notifications/rules', { site_id: site.id, ...b }),
    onSuccess: () => {
      toast.success(t('adm.rule.added'))
      qc.invalidateQueries({ queryKey: ['rules', site.id] })
      onDone()
    },
  })
}

/** Segmented channel picker (E-mail / WhatsApp / channels added later). */
export function ChannelPicker<C extends string = Channel>({ value, onChange, channels }: { value: C; onChange: (c: C) => void; channels: C[] }) {
  const { lang, t } = useI18n()
  return (
    <div role="radiogroup" aria-label={t('rule.channel')} className="inline-flex rounded-lg bg-gray-100 p-0.5">
      {channels.map((c) => {
        const Icon = channelIcon(c)
        const on = c === value
        return (
          <button
            key={c}
            type="button"
            role="radio"
            aria-checked={on}
            onClick={() => onChange(c)}
            className={`inline-flex h-8 items-center gap-1.5 rounded-md px-3 text-sm font-medium transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 ${
              on ? 'bg-white text-gray-900 shadow-card' : 'text-gray-600 hover:text-gray-900'
            }`}
          >
            <Icon className={`h-4 w-4 ${on ? channelTone(c) : ''}`} aria-hidden="true" />
            {translateMaybe(lang, `channel.${c}`, c)}
          </button>
        )
      })}
    </div>
  )
}

function recipientHint(t: ReturnType<typeof useI18n>['t'], ch: string) {
  return ch === 'email' ? t('adm.rule.hintEmail') : ch === 'whatsapp' ? t('adm.rule.hintWa') : t('p2.ch.hintOther')
}

/** Inline "add recipient" form inside one event card. */
function InlineAdd({ site, eventType, channels, onClose }: { site: Site; eventType: string; channels: string[]; onClose: () => void }) {
  const { t, lang } = useI18n()
  const [channel, setChannel] = useState<string>(eventType === 'sos' && channels.includes('whatsapp') ? 'whatsapp' : channels.includes('email') ? 'email' : (channels[0] ?? 'email'))
  const [recipient, setRecipient] = useState('')
  const [err, setErr] = useState('')
  const add = useAddRule(site, onClose)
  const inputId = useId()
  const hintId = useId()

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const r = normaliseRecipient(channel, recipient)
    if (!validRecipient(channel, r)) return setErr(recipientHint(t, channel))
    setErr('')
    add.mutate({ event_type: eventType, channel, recipient: r })
  }

  return (
    <form onSubmit={submit} noValidate className="mt-3 rounded-xl border border-gray-200 bg-gray-50/70 p-3">
      <div className="flex flex-wrap items-start gap-2">
        <ChannelPicker value={channel} onChange={setChannel} channels={channels} />
        <div className="relative min-w-[12rem] flex-1">
          <label htmlFor={inputId} className="sr-only">
            {t('rule.recipient')}
          </label>
          <Input
            id={inputId}
            autoFocus
            type={channel === 'email' ? 'email' : channel === 'whatsapp' ? 'tel' : 'text'}
            inputMode={channel === 'email' ? 'email' : channel === 'whatsapp' ? 'tel' : 'text'}
            placeholder={channel === 'email' ? t('plat.phEmail') : channel === 'whatsapp' ? t('plat.phWa') : undefined}
            value={recipient}
            onChange={(e) => setRecipient(e.target.value)}
            aria-invalid={err || add.error ? true : undefined}
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
      <p id={hintId} className={`mt-2 text-xs ${err || add.error ? 'font-medium text-red-700' : 'text-gray-500'}`} role={err || add.error ? 'alert' : undefined}>
        {err || (add.error ? errorText(add.error, lang) : recipientHint(t, channel))}
      </p>
    </form>
  )
}

/** Full "add recipient" dialog (header button and ?new=1 deep link). */
function AddRuleModal({ site, events, channels, onClose }: { site: Site; events: string[]; channels: string[]; onClose: () => void }) {
  const { t, lang } = useI18n()
  const [eventType, setEventType] = useState(events[0] ?? 'sos')
  const [channel, setChannel] = useState<string>(channels.includes('whatsapp') ? 'whatsapp' : (channels[0] ?? 'email'))
  const [recipient, setRecipient] = useState('')
  const [err, setErr] = useState('')
  const add = useAddRule(site, onClose)
  const meta = EVENT_META[eventType]

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const r = normaliseRecipient(channel, recipient)
    if (!validRecipient(channel, r)) return setErr(recipientHint(t, channel))
    setErr('')
    add.mutate({ event_type: eventType, channel, recipient: r })
  }

  return (
    <Modal
      open
      onClose={onClose}
      title={t('adm.rule.addTitle')}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="add-rule-form" loading={add.isPending}>
            <Plus className="h-4 w-4" aria-hidden="true" />
            {t('adm.rule.add')}
          </Button>
        </>
      }
    >
      <form id="add-rule-form" onSubmit={submit} noValidate className="space-y-4">
        <FormError message={add.error ? errorText(add.error, lang) : null} />
        <Field label={t('adm.rule.when')} required hint={meta ? t(meta.desc) : undefined}>
          <Select value={eventType} onChange={(e) => setEventType(e.target.value)}>
            {events.map((ev) => (
              <option key={ev} value={ev}>
                {EVENT_META[ev] ? t(EVENT_META[ev].title) : translateMaybe(lang, `event.${ev}`, ev)}
              </option>
            ))}
          </Select>
        </Field>
        <div>
          <p className="mb-1.5 text-sm font-medium text-gray-700">{t('adm.rule.via')}</p>
          <ChannelPicker value={channel} onChange={setChannel} channels={channels} />
        </div>
        <Field label={t('adm.rule.to')} required hint={recipientHint(t, channel)} error={err || undefined}>
          <Input
            type={channel === 'email' ? 'email' : channel === 'whatsapp' ? 'tel' : 'text'}
            inputMode={channel === 'email' ? 'email' : channel === 'whatsapp' ? 'tel' : 'text'}
            placeholder={channel === 'email' ? t('plat.phEmail') : channel === 'whatsapp' ? t('plat.phWa') : undefined}
            value={recipient}
            onChange={(e) => setRecipient(e.target.value)}
            autoComplete="off"
          />
        </Field>
      </form>
    </Modal>
  )
}

type ChipRule = Pick<NotificationRule, 'recipient' | 'enabled'> & { channel: string }

export function RecipientChip({ rule, onToggle, onRemove, busy }: { rule: ChipRule; onToggle: () => void; onRemove: () => void; busy: boolean }) {
  const { t, lang } = useI18n()
  const Icon = channelIcon(rule.channel)
  const shown = displayRecipient(rule.channel, rule.recipient)
  const channelName = translateMaybe(lang, `channel.${rule.channel}`, rule.channel)
  return (
    <li
      className={`relative inline-flex max-w-full items-center gap-2 rounded-full border py-1 pl-2.5 pr-1 text-sm transition-colors ${
        rule.enabled ? 'border-gray-200 bg-white text-gray-800 shadow-sm' : 'border-dashed border-gray-300 bg-gray-50 text-gray-600'
      }`}
    >
      <Icon className={`h-4 w-4 shrink-0 ${rule.enabled ? channelTone(rule.channel) : 'text-gray-500'}`} aria-hidden="true" />
      <span className={`truncate tabular-nums ${rule.enabled ? '' : 'line-through'}`}>
        <span className="sr-only">{channelName}: </span>
        {shown}
      </span>
      <Switch
        size="sm"
        checked={rule.enabled}
        disabled={busy}
        onChange={onToggle}
        label={rule.enabled ? t('adm.rule.pause', { to: shown }) : t('adm.rule.resume', { to: shown })}
      />
      <button
        type="button"
        onClick={onRemove}
        aria-label={t('adm.rule.remove', { to: shown })}
        title={t('common.delete')}
        className="rounded-full p-1 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
      >
        <X className="h-3.5 w-3.5" aria-hidden="true" />
      </button>
    </li>
  )
}

function RulesForSite({ site }: { site: Site }) {
  const { t, lang } = useI18n()
  const qc = useQueryClient()
  const toast = useToast()
  const onError = useErrorToast()
  const [adding, setAdding] = useState<string | null>(null)
  const [modal, setModal] = useState(false)
  const [removing, setRemoving] = useState<NotificationRule | null>(null)
  useNewParam(() => setModal(true))

  const types = useEventTypes()
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['rules', site.id],
    queryFn: () => apiGet<NotificationRule[]>('/notifications/rules', { site_id: site.id }),
  })
  const { can } = useAuth()
  const channelsQ = useChannels()

  const toggle = useMutation({
    mutationFn: (r: NotificationRule) => apiClient.put(`/notifications/rules/${r.id}`, { enabled: !r.enabled }),
    onSuccess: (_d, r) => {
      toast.success(r.enabled ? t('adm.rule.paused') : t('adm.rule.resumed'))
      qc.invalidateQueries({ queryKey: ['rules', site.id] })
    },
    onError,
  })
  const del = useMutation({
    mutationFn: (id: string) => apiClient.delete(`/notifications/rules/${id}`),
    onSuccess: () => {
      toast.success(t('common.deleted'))
      qc.invalidateQueries({ queryKey: ['rules', site.id] })
      setRemoving(null)
    },
    onError,
  })

  const rules = data ?? []
  const events = sortEvents(Array.from(new Set([...(types.data?.event_types ?? FALLBACK_EVENTS), ...rules.map((r) => r.event_type)])))
  const channels: string[] = channelsQ.data?.length ? channelsQ.data.map((c) => c.name) : types.data?.channels?.length ? types.data.channels : ['email', 'whatsapp']
  const configured = new Map((channelsQ.data ?? []).map((c) => [c.name, c.configured]))
  const unconfigured = channels.filter((c) => configured.get(c) === false && rules.some((r) => r.channel === c && r.enabled))
  const covered = new Set(rules.filter((r) => r.enabled).map((r) => r.event_type)).size

  return (
    <>
      <SectionHeader
        title={t('adm.set.rules')}
        description={t('adm.rule.intro', { site: site.name })}
        actions={
          <Button onClick={() => setModal(true)}>
            <Plus className="h-4 w-4" aria-hidden="true" />
            {t('adm.rule.add')}
          </Button>
        }
      />

      {unconfigured.length > 0 && (
        <Notice tone="warn">
          <span className="font-medium">{t('adm.rule.channelMissing', { channels: unconfigured.map((c) => translateMaybe(lang, `channel.${c}`, c)).join(' & ') })}</span>{' '}
          {can('managePlatform') ? (
            <Link to="/settings?tab=channels" className="font-semibold underline underline-offset-2 hover:text-amber-950">
              {t('adm.rule.channelMissingCta')}
            </Link>
          ) : (
            t('p2.ch.askOperator')
          )}
        </Notice>
      )}

      {channelsQ.data && channelsQ.data.length > 0 && (
        <ul className="mb-4 flex flex-wrap items-center gap-2 text-xs" aria-label={t('p2.ch.title')}>
          <li className="font-medium text-gray-600">{t('p2.ch.title')}:</li>
          {channelsQ.data.map((c) => {
            const Icon = channelIcon(c.name)
            return (
              <li key={c.name} className="inline-flex items-center gap-1.5 rounded-full bg-white px-2.5 py-1 text-gray-700 ring-1 ring-inset ring-gray-200">
                <Icon className={`h-3.5 w-3.5 ${channelTone(c.name)}`} aria-hidden="true" />
                <span className="font-medium">{translateMaybe(lang, `channel.${c.name}`, c.name.replace(/[_-]+/g, ' '))}</span>
                {c.configured ? (
                  <span className="inline-flex items-center gap-0.5 text-emerald-700">
                    <CheckCircle2 className="h-3.5 w-3.5" aria-hidden="true" />
                    {t('p2.ch.ready')}
                  </span>
                ) : (
                  <span className="inline-flex items-center gap-0.5 text-amber-800">
                    <XCircle className="h-3.5 w-3.5" aria-hidden="true" />
                    {t('p2.ch.notSet')}
                  </span>
                )}
              </li>
            )
          })}
        </ul>
      )}

      {isLoading ? (
        <TableSkeleton />
      ) : error ? (
        <ErrorState error={error} onRetry={() => refetch()} />
      ) : (
        <>
          {rules.length === 0 ? (
            <div className="mb-4 flex flex-wrap items-center gap-4 rounded-2xl border border-primary-100 bg-gradient-to-r from-primary-50 via-white to-white p-4 pr-5 shadow-card">
              <span className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-primary-600 text-white shadow-lg shadow-primary-600/25">
                <BellRing className="h-5 w-5" aria-hidden="true" />
              </span>
              <div className="min-w-0 flex-1">
                <p className="text-sm font-semibold text-gray-900">{t('adm.rule.emptyTitle')}</p>
                <p className="mt-0.5 text-sm text-gray-600">{t('adm.rule.empty')}</p>
              </div>
              <Button onClick={() => setAdding('sos')}>
                <Plus className="h-4 w-4" aria-hidden="true" />
                {t('adm.rule.startSos')}
              </Button>
            </div>
          ) : (
            <p className="mb-3 text-sm text-gray-600">{t('adm.rule.coverage', { n: covered, total: events.length })}</p>
          )}

          <ul className="space-y-3">
            {events.map((ev) => {
              const meta = EVENT_META[ev]
              const list = rules.filter((r) => r.event_type === ev).sort((a, b) => a.channel.localeCompare(b.channel) || a.recipient.localeCompare(b.recipient))
              const active = list.filter((r) => r.enabled).length
              const title = meta ? t(meta.title) : translateMaybe(lang, `event.${ev}`, ev)
              return (
                <li key={ev} className="rounded-2xl border border-gray-200/80 bg-white p-4 shadow-card sm:p-5">
                  <div className="flex gap-3.5">
                    {meta && <IconTile icon={meta.icon} tone={meta.tone} />}
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
                        <h3 className="text-[15px] font-semibold text-gray-900">{title}</h3>
                        <span className={`text-xs font-medium ${active ? 'text-gray-600' : 'text-gray-500'}`}>
                          {active ? t('adm.rule.recipients', { n: active }) : t('adm.rule.noRecipients')}
                        </span>
                      </div>
                      {meta && <p className="mt-0.5 text-sm text-gray-600">{t(meta.desc)}</p>}

                      <ul className="mt-3 flex flex-wrap items-center gap-2" aria-label={t('adm.rule.recipientsOf', { event: title })}>
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
                              className={buttonClass('ghost', 'sm', 'rounded-full border border-dashed border-gray-300 text-gray-600 hover:border-primary-300 hover:text-primary-700')}
                              aria-label={`${t('adm.rule.addShort')} — ${title}`}
                            >
                              <Plus className="h-3.5 w-3.5" aria-hidden="true" />
                              {t('adm.rule.addShort')}
                            </button>
                          </li>
                        )}
                      </ul>

                      {list.length === 0 && adding !== ev && ev === 'sos' && (
                        <p className="mt-2 inline-flex items-center gap-1.5 text-xs font-medium text-amber-700">
                          <BellOff className="h-3.5 w-3.5" aria-hidden="true" />
                          {t('adm.rule.sosWarn')}
                        </p>
                      )}
                      {adding === ev && <InlineAdd site={site} eventType={ev} channels={channels} onClose={() => setAdding(null)} />}
                    </div>
                  </div>
                </li>
              )
            })}
          </ul>
        </>
      )}

      {modal && <AddRuleModal site={site} events={events} channels={channels} onClose={() => setModal(false)} />}
      <ConfirmDialog
        open={!!removing}
        title={t('adm.rule.removeTitle')}
        message={
          removing
            ? t('adm.rule.removeBody', {
                to: displayRecipient(removing.channel, removing.recipient),
                event: EVENT_META[removing.event_type] ? t(EVENT_META[removing.event_type].title) : removing.event_type,
              })
            : ''
        }
        confirmLabel={t('common.delete')}
        loading={del.isPending}
        onConfirm={() => removing && del.mutate(removing.id)}
        onClose={() => setRemoving(null)}
      />
    </>
  )
}

export function RulesTab() {
  const { t } = useI18n()
  const { sites, isLoading, error } = useSite()
  if (!isLoading && !error && sites.length === 0) {
    return (
      <>
        <SectionHeader title={t('adm.set.rules')} description={t('adm.rule.introNoSite')} />
        <EmptyState icon={Building2} title={t('adm.rule.noSiteTitle')} message={t('adm.rule.noSite')}>
          <Link to="/settings?tab=sites&new=1" className={buttonClass('primary', 'md')}>
            <Plus className="h-4 w-4" aria-hidden="true" />
            {t('adm.site.new')}
          </Link>
        </EmptyState>
      </>
    )
  }
  return <RequireSite>{(site) => <RulesForSite key={site.id} site={site} />}</RequireSite>
}
