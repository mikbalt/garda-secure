import { useState, type ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import { ArrowRight, BellRing, Building2, CalendarClock, Clock, QrCode, ShieldCheck, Tablet, UserPlus } from 'lucide-react'
import { useSite } from '../context/site'
import { useAuth } from '../hooks/useAuth'
import { useI18n, type Key } from '../i18n/context'
import { SiteModal } from '../pages/settings/SiteModal'
import { TableSkeleton } from './LoadingSkeleton'
import { ErrorState } from './ui/States'
import { Button } from './ui/Button'
import type { Site } from '../types'

const STEPS: { icon: typeof Building2; title: Key }[] = [
  { icon: Building2, title: 'setup.step.site.title' },
  { icon: UserPlus, title: 'setup.step.guards.title' },
  { icon: CalendarClock, title: 'setup.step.schedule.title' },
  { icon: QrCode, title: 'setup.step.routes.title' },
  { icon: Tablet, title: 'setup.step.device.title' },
  { icon: BellRing, title: 'setup.step.notif.title' },
]

/** First-run screen: the organisation has no location yet. */
function Welcome() {
  const { t } = useI18n()
  const { can } = useAuth()
  const navigate = useNavigate()
  const [creating, setCreating] = useState(false)
  const admin = can('manageSites')

  return (
    <div className="mx-auto max-w-5xl">
      <div className="relative overflow-hidden rounded-3xl bg-ink-900 px-6 py-10 text-white shadow-overlay sm:px-10 sm:py-12">
        <div className="pointer-events-none absolute -right-24 -top-24 h-80 w-80 rounded-full bg-primary-500/30 blur-3xl" aria-hidden="true" />
        <div className="pointer-events-none absolute -bottom-32 left-1/3 h-72 w-72 rounded-full bg-emerald-500/15 blur-3xl" aria-hidden="true" />
        <div className="relative grid gap-10 lg:grid-cols-[1.1fr_1fr] lg:items-center">
          <div>
            <div className="mb-5 inline-flex h-12 w-12 items-center justify-center rounded-2xl bg-gradient-to-br from-primary-400 to-primary-700 shadow-lg shadow-primary-900/50 ring-1 ring-white/20">
              <ShieldCheck className="h-6 w-6" aria-hidden="true" />
            </div>
            <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">{t('welcome.title')}</h1>
            <p className="mt-3 max-w-lg text-base leading-relaxed text-gray-300">{admin ? t('welcome.body') : t('welcome.noAccess')}</p>
            {admin && (
              <div className="mt-7 flex flex-wrap items-center gap-4">
                <Button size="lg" onClick={() => setCreating(true)} className="!bg-white !text-gray-900 hover:!bg-gray-100">
                  {t('welcome.cta')}
                  <ArrowRight className="h-4 w-4" aria-hidden="true" />
                </Button>
                <span className="inline-flex items-center gap-1.5 text-sm text-gray-400">
                  <Clock className="h-4 w-4" aria-hidden="true" />
                  {t('welcome.minutes')}
                </span>
              </div>
            )}
          </div>
          <div className="rounded-2xl bg-white/[0.04] p-5 ring-1 ring-white/10 backdrop-blur">
            <p className="mb-3 text-xs font-semibold uppercase tracking-wider text-gray-400">{t('welcome.stepsTitle')}</p>
            <ol className="space-y-2.5">
              {STEPS.map(({ icon: Icon, title }, i) => (
                <li key={title} className="flex items-center gap-3">
                  <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-white/[0.07] text-primary-200 ring-1 ring-white/10">
                    <Icon className="h-4 w-4" aria-hidden="true" />
                  </span>
                  <span className="text-sm text-gray-200">
                    <span className="mr-1.5 tabular-nums text-gray-500">{i + 1}.</span>
                    {t(title)}
                  </span>
                </li>
              ))}
            </ol>
          </div>
        </div>
      </div>
      {creating && <SiteModal site={null} onClose={() => setCreating(false)} onSaved={() => navigate('/setup')} />}
    </div>
  )
}

/**
 * Renders children with the selected site, or a loading / error / first-run
 * welcome state (admins can create the first location right there).
 */
export function RequireSite({ children }: { children: (site: Site) => ReactNode }) {
  const { site, isLoading, error } = useSite()
  if (isLoading) return <TableSkeleton />
  if (error) return <ErrorState error={error} />
  if (!site) return <Welcome />
  return <>{children(site)}</>
}
