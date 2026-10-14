import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Monitor, Smartphone, TabletSmartphone, Trash2 } from 'lucide-react'
import apiClient, { apiErrorStatus } from '../../api/client'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Card, EmptyState, ErrorState } from '../../components/ui/States'
import { useToast } from '../../components/toast/context'
import { PUSH_KEYS, type usePush } from '../../hooks/usePush'
import { useI18n } from '../../i18n/context'
import { formatDateTime } from '../../lib/time'
import { ConfirmDialog, IconTile } from '../settings/kit'
import { deliveryErrorText, relativeTime } from '../settings/format'
import { pushErrorText } from './pushErrorText'
import type { PushSubscriptionRow } from '../../types/push'

type Push = ReturnType<typeof usePush>

const browserTz = () => {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'Asia/Jakarta'
  } catch {
    return 'Asia/Jakarta'
  }
}

/** The user's devices that receive push (GET /push/subscriptions), with remove. */
export function DeviceList({ push }: { push: Push }) {
  const { t, lang } = useI18n()
  const qc = useQueryClient()
  const toast = useToast()
  const [removing, setRemoving] = useState<PushSubscriptionRow | null>(null)
  const tz = browserTz()
  const now = Date.now()

  const remove = useMutation({
    mutationFn: async (row: PushSubscriptionRow) => {
      // This browser: also unsubscribe locally, otherwise only the server row goes.
      if (row.endpoint === push.endpoint) return push.disable.mutateAsync()
      try {
        await apiClient.delete(`/push/subscriptions/${encodeURIComponent(row.id)}`)
      } catch (e) {
        if (apiErrorStatus(e) !== 404) throw e
      }
    },
    onSuccess: () => {
      toast.success(t('push.devices.removed'))
      setRemoving(null)
    },
    onError: (e) => toast.error(pushErrorText(e, lang, t)),
    onSettled: () => qc.invalidateQueries({ queryKey: PUSH_KEYS.subscriptions }),
  })

  const rows = [...push.rows].sort((a, b) => Number(b.endpoint === push.endpoint) - Number(a.endpoint === push.endpoint) || b.created_at.localeCompare(a.created_at))
  const q = push.rowsQuery

  return (
    <Card title={t('push.devices.title')} description={t('push.devices.desc')} flush>
      {q.isLoading ? (
        <div className="space-y-3 p-5" aria-hidden="true">
          {[0, 1].map((i) => (
            <div key={i} className="h-10 animate-pulse rounded-lg bg-gray-100" />
          ))}
        </div>
      ) : q.error ? (
        <div className="p-5">
          <ErrorState error={q.error} onRetry={() => q.refetch()} />
        </div>
      ) : rows.length === 0 ? (
        <EmptyState bare message={t('push.devices.empty')} />
      ) : (
        <ul className="divide-y divide-gray-100">
          {rows.map((r) => {
            const mine = !!push.endpoint && r.endpoint === push.endpoint
            const label = r.label || (r.kind === 'webpush' ? '—' : t('push.devices.app'))
            const icon = r.kind !== 'webpush' ? TabletSmartphone : /iPhone|iPad|Android/.test(r.label) ? Smartphone : Monitor
            const failedLast = !!r.last_failure_at && (!r.last_success_at || Date.parse(r.last_failure_at) > Date.parse(r.last_success_at))
            return (
              <li key={r.id} className="flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3 sm:px-5">
                <div className="flex min-w-0 flex-1 items-start gap-3">
                  <IconTile icon={icon} tone={mine ? 'blue' : 'gray'} size="sm" />
                  <div className="min-w-0">
                    <p className="flex flex-wrap items-center gap-2 text-sm font-medium text-gray-900">
                      <span className="break-words">{label}</span>
                      {mine && <Badge tone="blue">{t('push.device.title')}</Badge>}
                    </p>
                    <p className="mt-0.5 flex flex-wrap gap-x-3 gap-y-0.5 text-xs text-gray-500">
                      <span title={formatDateTime(r.created_at, tz, lang)}>{t('push.devices.added', { when: relativeTime(r.created_at, lang, now) })}</span>
                      <span title={r.last_success_at ? formatDateTime(r.last_success_at, tz, lang) : undefined}>
                        {r.last_success_at ? t('push.devices.lastOk', { when: relativeTime(r.last_success_at, lang, now) }) : t('push.devices.never')}
                      </span>
                    </p>
                    {failedLast && r.last_error && (
                      <p className="mt-0.5 break-words text-xs font-medium text-red-700">
                        {t('push.devices.lastError', { when: relativeTime(r.last_failure_at, lang, now), error: deliveryErrorText(r.last_error, lang) })}
                      </p>
                    )}
                  </div>
                </div>
                <Button
                  size="sm"
                  variant="ghost"
                  className="text-red-700 hover:bg-red-50 hover:text-red-800"
                  onClick={() => setRemoving(r)}
                  aria-label={t('push.devices.removeLabel', { label })}
                >
                  <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
                  {t('push.devices.remove')}
                </Button>
              </li>
            )
          })}
        </ul>
      )}
      <ConfirmDialog
        open={!!removing}
        title={t('push.devices.removeTitle')}
        message={removing ? t('push.devices.removeBody', { label: removing.label || '—' }) : ''}
        confirmLabel={t('push.devices.remove')}
        loading={remove.isPending}
        onConfirm={() => removing && remove.mutate(removing)}
        onClose={() => setRemoving(null)}
      />
    </Card>
  )
}
