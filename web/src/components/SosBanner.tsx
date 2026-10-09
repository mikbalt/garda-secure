import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Siren, MapPin } from 'lucide-react'
import apiClient, { apiGet } from '../api/client'
import { useSite } from '../context/site'
import { useAuth } from '../hooks/useAuth'
import { useI18n } from '../i18n/context'
import { formatDateTime, mapsUrl } from '../lib/time'
import { useErrorToast, useToast } from './toast/context'
import { Button } from './ui/Button'
import type { SosAlert } from '../types'

/**
 * Prominent banner for unacknowledged SOS alerts of the selected site.
 * Polls every 10 s (admin / supervisor only).
 */
export function SosBanner() {
  const { can } = useAuth()
  const { site, siteId } = useSite()
  const { t, lang } = useI18n()
  const qc = useQueryClient()
  const toast = useToast()
  const onError = useErrorToast()
  const enabled = can('viewSos') && !!siteId

  const { data } = useQuery({
    queryKey: ['sos', siteId, 'unack'],
    queryFn: () => apiGet<SosAlert[]>('/sos', { site_id: siteId, unack_only: true }),
    enabled,
    refetchInterval: 10_000,
    refetchIntervalInBackground: true,
  })

  const ack = useMutation({
    mutationFn: (id: string) => apiClient.post(`/sos/${id}/acknowledge`),
    onSuccess: () => {
      toast.success(t('sos.ackDone'))
      qc.invalidateQueries({ queryKey: ['sos'] })
      qc.invalidateQueries({ queryKey: ['dashboard'] })
    },
    onError,
  })

  const alerts = enabled ? data ?? [] : []
  const tz = site?.timezone ?? 'Asia/Jakarta'

  return (
    <div role="alert" aria-live="assertive" className="no-print">
      {alerts.length > 0 && (
        <div className="relative overflow-hidden bg-gradient-to-r from-red-600 via-red-600 to-rose-600 px-4 py-3 text-white shadow-[inset_0_-1px_0_rgba(0,0,0,0.15)] sm:px-6">
          <div className="mx-auto flex max-w-[1400px] flex-col gap-2 lg:px-2">
            <div className="flex items-center gap-2.5 text-sm font-semibold">
              <span className="relative flex h-7 w-7 items-center justify-center rounded-full bg-white/15">
                <span className="absolute inline-flex h-full w-full animate-ping2 rounded-full bg-white/30" />
                <Siren className="relative h-4 w-4" aria-hidden="true" />
              </span>
              <span>
                {t('sos.bannerTitle')} · {t('sos.bannerCount', { n: alerts.length })}
              </span>
            </div>
            <ul className="space-y-1.5">
              {alerts.map((a) => (
                <li key={a.id} className="flex flex-wrap items-center gap-x-4 gap-y-1 rounded-xl bg-black/15 px-3.5 py-2 text-sm ring-1 ring-white/10">
                  <span className="font-semibold">{t('sos.firedBy', { guard: a.guard_name || '—' })}</span>
                  <span className="text-white/80">{formatDateTime(a.fired_at, tz, lang)}</span>
                  {a.gps_lat != null && a.gps_lng != null ? (
                    <a
                      href={mapsUrl(a.gps_lat, a.gps_lng)}
                      target="_blank"
                      rel="noreferrer"
                      className="inline-flex items-center gap-1 text-white/90 underline decoration-white/40 underline-offset-2 hover:text-white focus:outline-none focus-visible:ring-2 focus-visible:ring-white"
                    >
                      <MapPin className="h-4 w-4" aria-hidden="true" />
                      {t('common.openMap')}
                    </a>
                  ) : (
                    <span className="text-white/70">{t('sos.noGps')}</span>
                  )}
                  <Button
                    size="sm"
                    variant="secondary"
                    className="ml-auto !text-red-700 !ring-white"
                    loading={ack.isPending && ack.variables === a.id}
                    onClick={() => ack.mutate(a.id)}
                  >
                    {t('sos.acknowledge')}
                  </Button>
                </li>
              ))}
            </ul>
          </div>
        </div>
      )}
    </div>
  )
}
