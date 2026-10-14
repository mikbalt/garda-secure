import { useId } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { SlidersHorizontal } from 'lucide-react'
import apiClient, { apiGet } from '../../api/client'
import { Card, EmptyState, ErrorState, Notice } from '../../components/ui/States'
import { useErrorToast } from '../../components/toast/context'
import { PUSH_KEYS } from '../../hooks/usePush'
import { useI18n, translateMaybe } from '../../i18n/context'
import { IconTile } from '../settings/kit'
import { PUSH_EVENT_ICON, groupAvailable } from './events'
import type { PushPreferences } from '../../types/push'

const SAVE_KEY = ['push', 'preferences', 'save'] as const

function humanise(ev: string) {
  return ev.replace(/[_-]+/g, ' ').replace(/^\w/, (c) => c.toUpperCase())
}

function PrefRow({ event, checked, onChange }: { event: string; checked: boolean; onChange: (v: boolean) => void }) {
  const { lang } = useI18n()
  const titleId = useId()
  const descId = useId()
  const meta = PUSH_EVENT_ICON[event]
  const title = translateMaybe(lang, `push.ev.${event}`, humanise(event))
  const desc = translateMaybe(lang, `push.ev.${event}.desc`, '')
  return (
    <li className="flex items-center gap-3 px-4 py-3 sm:px-5">
      {meta && <IconTile icon={meta.icon} tone={meta.tone} size="sm" />}
      <div className="min-w-0 flex-1">
        <p id={titleId} className="text-sm font-medium text-gray-900">
          {title}
        </p>
        {desc && (
          <p id={descId} className="mt-0.5 text-xs text-gray-500">
            {desc}
          </p>
        )}
      </div>
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        aria-labelledby={titleId}
        aria-describedby={desc ? descId : undefined}
        data-event={event}
        onClick={() => onChange(!checked)}
        className={`relative inline-flex h-6 w-11 shrink-0 items-center rounded-full p-0.5 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-2 ${
          checked ? 'bg-emerald-600' : 'bg-gray-300'
        }`}
      >
        <span className={`h-5 w-5 rounded-full bg-white shadow-sm transition-transform ${checked ? 'translate-x-5' : 'translate-x-0'}`} aria-hidden="true" />
      </button>
    </li>
  )
}

/**
 * Which push alerts the user wants (GET/PUT /push/preferences): only the
 * events the user's roles can receive (`available`), grouped; saved per
 * switch, optimistically.
 */
export function Preferences({ hasDevice }: { hasDevice: boolean }) {
  const { t } = useI18n()
  const qc = useQueryClient()
  const onError = useErrorToast()
  const q = useQuery({ queryKey: PUSH_KEYS.preferences, queryFn: () => apiGet<PushPreferences>('/push/preferences') })

  const save = useMutation({
    mutationKey: SAVE_KEY,
    mutationFn: async (events: Record<string, boolean>) => (await apiClient.put<PushPreferences>('/push/preferences', { events })).data,
    onMutate: async (events) => {
      await qc.cancelQueries({ queryKey: PUSH_KEYS.preferences })
      const prev = qc.getQueryData<PushPreferences>(PUSH_KEYS.preferences)
      if (prev) qc.setQueryData<PushPreferences>(PUSH_KEYS.preferences, { ...prev, events: { ...prev.events, ...events } })
      return { prev }
    },
    onError: (err, _events, ctx) => {
      if (ctx?.prev) qc.setQueryData(PUSH_KEYS.preferences, ctx.prev)
      onError(err)
    },
    onSuccess: (data) => {
      // A later toggle may still be in flight: keep its optimistic value until it answers.
      if (data?.events && qc.isMutating({ mutationKey: SAVE_KEY }) <= 1) qc.setQueryData(PUSH_KEYS.preferences, data)
    },
  })

  const prefs = q.data
  const groups = prefs ? groupAvailable(prefs.available ?? []) : []

  return (
    <Card title={t('push.prefs.title')} description={t('push.prefs.desc')} icon={SlidersHorizontal} flush>
      {q.isLoading ? (
        <div className="space-y-3 p-5" aria-hidden="true">
          {[0, 1, 2, 3].map((i) => (
            <div key={i} className="h-10 animate-pulse rounded-lg bg-gray-100" />
          ))}
        </div>
      ) : q.error ? (
        <div className="p-5">
          <ErrorState error={q.error} onRetry={() => q.refetch()} />
        </div>
      ) : groups.length === 0 ? (
        <EmptyState bare message={t('push.prefs.none')} />
      ) : (
        <>
          {!hasDevice && (
            <div className="px-4 pt-4 sm:px-5 [&>div]:mb-0">
              <Notice>{t('push.prefs.noDevice')}</Notice>
            </div>
          )}
          {groups.map((g) => (
            <section key={g.key} aria-labelledby={`push-group-${g.key}`} className="border-t border-gray-100 first:border-t-0">
              <h4 id={`push-group-${g.key}`} className="px-4 pb-1 pt-4 text-xs font-semibold uppercase tracking-wider text-gray-500 sm:px-5">
                {t(g.label)}
              </h4>
              <ul className="divide-y divide-gray-100">
                {g.events.map((ev) => (
                  <PrefRow key={ev} event={ev} checked={!!prefs?.events?.[ev]} onChange={(v) => save.mutate({ [ev]: v })} />
                ))}
              </ul>
            </section>
          ))}
        </>
      )}
    </Card>
  )
}
