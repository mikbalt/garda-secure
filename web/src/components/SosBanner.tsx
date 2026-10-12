import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Bell, BellRing, MapPin, Siren, Volume2, VolumeX } from 'lucide-react'
import apiClient from '../api/client'
import { useSite } from '../context/site'
import { useActiveSos } from '../hooks/useActiveSos'
import { useAuth } from '../hooks/useAuth'
import { useI18n } from '../i18n/context'
import { beep, flashTitle, notificationPermission, notify, requestNotifications, unlockAudio } from '../lib/alarm'
import { readStored, STORAGE_KEYS, writeStored } from '../lib/storage'
import { formatDateTime, mapsUrl } from '../lib/time'
import { useErrorToast, useToast } from './toast/context'
import { Button } from './ui/Button'
import type { SosAlert } from '../types'

const BEEP_EVERY_MS = 3_000
const FLASH_EVERY_MS = 1_000
const MAX_SHOWN = 5

/** Alarm side effects: beep until acknowledged or muted, flash the title, notify new alerts. */
function useAlarm(alerts: SosAlert[], muted: boolean, describe: (a: SosAlert) => { title: string; body: string }) {
  const count = alerts.length
  const seen = useRef<Set<string>>(new Set())
  const [announce, setAnnounce] = useState('')
  const describeRef = useRef(describe)
  describeRef.current = describe

  // Browsers only allow audio after a user gesture: unlock on the first one.
  useEffect(() => {
    const unlock = () => unlockAudio()
    window.addEventListener('pointerdown', unlock, { passive: true })
    window.addEventListener('keydown', unlock)
    return () => {
      window.removeEventListener('pointerdown', unlock)
      window.removeEventListener('keydown', unlock)
    }
  }, [])

  useEffect(() => {
    if (count === 0 || muted) return
    beep()
    const id = window.setInterval(beep, BEEP_EVERY_MS)
    return () => window.clearInterval(id)
  }, [count, muted])

  useEffect(() => {
    if (count === 0) return
    const base = document.title
    let on = false
    const id = window.setInterval(() => {
      on = !on
      document.title = on ? flashTitle(count, base) : base
    }, FLASH_EVERY_MS)
    return () => {
      window.clearInterval(id)
      document.title = base
    }
  }, [count])

  // New alerts: announce once (text only) and raise a desktop notification.
  useEffect(() => {
    const fresh = alerts.filter((a) => !seen.current.has(a.id))
    seen.current = new Set(alerts.map((a) => a.id))
    if (fresh.length === 0) return
    const latest = describeRef.current(fresh[0])
    setAnnounce(`${latest.title}. ${latest.body}`)
    for (const a of fresh) {
      const d = describeRef.current(a)
      notify(d.title, d.body, `sos-${a.id}`)
    }
  }, [alerts])

  return announce
}

/**
 * Banner for unacknowledged SOS alerts of **all** sites the user can see
 * (WEB-3), polled every 10 s for admin / supervisor, with sound, title
 * flashing and optional desktop notifications. The live region only carries
 * text; the controls sit outside it.
 */
export function SosBanner() {
  const { can } = useAuth()
  const { sites } = useSite()
  const { t, lang } = useI18n()
  const qc = useQueryClient()
  const toast = useToast()
  const onError = useErrorToast()
  const { data } = useActiveSos()
  const [muted, setMutedState] = useState(() => readStored(STORAGE_KEYS.sosMuted) === '1')
  const [perm, setPerm] = useState(notificationPermission)

  const alerts = can('viewSos') ? data ?? [] : []
  const siteOf = (a: SosAlert) => sites.find((s) => s.id === a.site_id)
  const multiSite = sites.length > 1
  const tzOf = (a: SosAlert) => siteOf(a)?.timezone ?? 'Asia/Jakarta'

  const describe = (a: SosAlert) => ({
    title: t('sos.notifyTitle', { site: siteOf(a)?.name ?? '—' }),
    body: t('sos.notifyBody', { guard: a.guard_name || '—', time: formatDateTime(a.fired_at, tzOf(a), lang) }),
  })
  const announce = useAlarm(alerts, muted, describe)

  const setMuted = (v: boolean) => {
    setMutedState(v)
    writeStored(STORAGE_KEYS.sosMuted, v ? '1' : null)
    if (!v) unlockAudio()
  }

  const ack = useMutation({
    mutationFn: (id: string) => apiClient.post(`/sos/${id}/acknowledge`),
    onSuccess: () => {
      toast.success(t('sos.ackDone'))
      qc.invalidateQueries({ queryKey: ['sos'] })
      qc.invalidateQueries({ queryKey: ['dashboard'] })
    },
    onError,
  })

  const shown = alerts.slice(0, MAX_SHOWN)

  return (
    <div className="no-print">
      {/* Screen readers: text only, announced when a new alert arrives. */}
      <div role="alert" className="sr-only">
        {alerts.length > 0 ? announce : ''}
      </div>
      {alerts.length > 0 && (
        <section
          aria-label={t('sos.bannerTitle')}
          className="relative overflow-hidden bg-gradient-to-r from-red-700 via-red-700 to-rose-700 px-4 py-3 text-white shadow-[inset_0_-1px_0_rgba(0,0,0,0.15)] sm:px-6"
        >
          <div className="mx-auto flex max-w-[1400px] flex-col gap-2 lg:px-2">
            <div className="flex flex-wrap items-center gap-2.5 text-sm font-semibold">
              <span className="relative flex h-7 w-7 items-center justify-center rounded-full bg-white/15">
                <span className="absolute inline-flex h-full w-full animate-ping2 rounded-full bg-white/30" aria-hidden="true" />
                <Siren className="relative h-4 w-4" aria-hidden="true" />
              </span>
              <h2>
                {t('sos.bannerTitle')} · {t('sos.bannerCount', { n: alerts.length })}
              </h2>
              <div className="ml-auto flex flex-wrap items-center gap-1.5">
                <button
                  type="button"
                  aria-pressed={muted}
                  onClick={() => setMuted(!muted)}
                  className="inline-flex h-8 items-center gap-1.5 rounded-lg bg-white/15 px-2.5 text-xs font-medium hover:bg-white/25 focus:outline-none focus-visible:ring-2 focus-visible:ring-white"
                >
                  {muted ? <VolumeX className="h-4 w-4" aria-hidden="true" /> : <Volume2 className="h-4 w-4" aria-hidden="true" />}
                  {muted ? t('sos.soundOff') : t('sos.soundOn')}
                </button>
                {perm === 'default' && (
                  <button
                    type="button"
                    onClick={async () => setPerm(await requestNotifications())}
                    className="inline-flex h-8 items-center gap-1.5 rounded-lg bg-white/15 px-2.5 text-xs font-medium hover:bg-white/25 focus:outline-none focus-visible:ring-2 focus-visible:ring-white"
                  >
                    <Bell className="h-4 w-4" aria-hidden="true" />
                    {t('sos.notifyEnable')}
                  </button>
                )}
                {perm === 'granted' && (
                  <span className="inline-flex items-center gap-1 text-xs font-medium text-white/90">
                    <BellRing className="h-3.5 w-3.5" aria-hidden="true" />
                    {t('sos.notifyOn')}
                  </span>
                )}
              </div>
            </div>
            <ul className="space-y-1.5">
              {shown.map((a) => (
                <li key={a.id} className="flex flex-wrap items-center gap-x-4 gap-y-1 rounded-xl bg-black/20 px-3.5 py-2 text-sm ring-1 ring-white/10">
                  <span className="font-semibold">{t('sos.firedBy', { guard: a.guard_name || '—' })}</span>
                  {multiSite && <span className="font-medium">{siteOf(a)?.name ?? '—'}</span>}
                  <span className="text-white">{formatDateTime(a.fired_at, tzOf(a), lang)}</span>
                  {a.gps_lat != null && a.gps_lng != null ? (
                    <a
                      href={mapsUrl(a.gps_lat, a.gps_lng)}
                      target="_blank"
                      rel="noreferrer"
                      className="inline-flex items-center gap-1 text-white underline decoration-white/60 underline-offset-2 focus:outline-none focus-visible:ring-2 focus-visible:ring-white"
                    >
                      <MapPin className="h-4 w-4" aria-hidden="true" />
                      {t('common.openMap')}
                    </a>
                  ) : (
                    <span className="text-white">{t('sos.noGps')}</span>
                  )}
                  <Button
                    size="sm"
                    variant="secondary"
                    className="ml-auto !text-red-800 !ring-white"
                    loading={ack.isPending && ack.variables === a.id}
                    onClick={() => ack.mutate(a.id)}
                    aria-label={`${t('sos.acknowledge')} — ${a.guard_name || '—'}${multiSite ? `, ${siteOf(a)?.name ?? ''}` : ''}`}
                  >
                    {t('sos.acknowledge')}
                  </Button>
                </li>
              ))}
            </ul>
            {alerts.length > shown.length && (
              <Link to="/incidents?tab=sos" className="self-start text-sm font-medium text-white underline underline-offset-2 focus:outline-none focus-visible:ring-2 focus-visible:ring-white">
                {t('sos.more', { n: alerts.length - shown.length })}
              </Link>
            )}
          </div>
        </section>
      )}
    </div>
  )
}
