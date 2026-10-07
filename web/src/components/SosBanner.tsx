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
        <div className="border-b-4 border-red-900 bg-red-700 px-4 py-3 text-white sm:px-6">
          <div className="flex items-center gap-2 font-semibold">
            <Siren className="h-5 w-5 animate-pulse" aria-hidden="true" />
            <span>
              {t('sos.bannerTitle')} — {t('sos.bannerCount', { n: alerts.length })}
            </span>
          </div>
          <ul className="mt-2 space-y-2">
            {alerts.map((a) => (
              <li key={a.id} className="flex flex-wrap items-center gap-x-4 gap-y-1 rounded-md bg-red-800/60 px-3 py-2 text-sm">
                <span className="font-semibold">{t('sos.firedBy', { guard: (a.guard_name || '—') })}</span>
                <span>{formatDateTime(a.fired_at, tz, lang)}</span>
                {a.gps_lat != null && a.gps_lng != null ? (
                  <a
                    href={mapsUrl(a.gps_lat, a.gps_lng)}
                    target="_blank"
                    rel="noreferrer"
                    className="inline-flex items-center gap-1 underline underline-offset-2 hover:text-red-100 focus:outline-none focus-visible:ring-2 focus-visible:ring-white"
                  >
                    <MapPin className="h-4 w-4" aria-hidden="true" />
                    {a.gps_lat.toFixed(5)}, {a.gps_lng.toFixed(5)}
                  </a>
                ) : (
                  <span className="text-red-100">{t('sos.noGps')}</span>
                )}
                <Button
                  size="sm"
                  variant="secondary"
                  className="ml-auto !text-red-800"
                  loading={ack.isPending && ack.variables === a.id}
                  onClick={() => ack.mutate(a.id)}
                >
                  {t('sos.acknowledge')}
                </Button>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}
