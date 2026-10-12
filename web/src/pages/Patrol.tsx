import { useQuery } from '@tanstack/react-query'
import { useSearchParams } from 'react-router-dom'
import { Plus, Route as RouteIcon } from 'lucide-react'
import { apiGet } from '../api/client'
import { RequireSite } from '../components/RequireSite'
import { Button } from '../components/ui/Button'
import { PageHeader } from '../components/ui/States'
import { TabPanel, Tabs, type TabItem } from '../components/ui/Tabs'
import { useAuth } from '../hooks/useAuth'
import { useTabParam } from '../hooks/useTabParam'
import { useSite } from '../context/site'
import { useT } from '../i18n/context'
import { LiveTab } from './patrol/LiveTab'
import { SessionsTab } from './patrol/SessionsTab'
import { RoutesTab } from './patrol/RoutesTab'
import type { LivePatrol } from '../types'

type TabKey = 'live' | 'sessions' | 'routes'

export default function Patrol() {
  const t = useT()
  const { can } = useAuth()
  const { siteId } = useSite()
  const [, setParams] = useSearchParams()
  // Route definitions (and QR sheets) are for operations staff (D-020); clients see live + history.
  const showRoutes = can('manageIncidents')
  const canEdit = can('editRoutes')

  // Shares the cache entry polled by the live tab and the dashboard.
  const live = useQuery({
    queryKey: ['patrol-live', siteId],
    queryFn: () => apiGet<LivePatrol[]>('/patrol/live', { site_id: siteId }),
    enabled: !!siteId,
    refetchInterval: 10_000,
  })
  const overdue = (live.data ?? []).filter((p) => p.progress.checkpoints.some((c) => c.status === 'overdue')).length

  const tabs: TabItem<TabKey>[] = [
    { key: 'live', label: t('pat.tabLive'), badge: overdue || undefined },
    { key: 'sessions', label: t('ops.pat.tabHistory') },
    ...(showRoutes ? [{ key: 'routes' as const, label: t('pat.tabRoutes') }] : []),
  ]
  const [tab, setTab] = useTabParam<TabKey>(
    tabs.map((x) => x.key),
    'live'
  )

  const newRoute = () =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev)
        next.set('tab', 'routes')
        next.set('new', '1')
        return next
      },
      { replace: true }
    )

  return (
    <div>
      <PageHeader
        icon={RouteIcon}
        title={t('pat.title')}
        subtitle={showRoutes ? t('ops.pat.subtitle') : t('ops.pat.subtitleRo')}
        actions={
          canEdit && tab === 'routes' && siteId ? (
            <Button onClick={newRoute}>
              <Plus className="h-4 w-4" aria-hidden="true" />
              {t('pat.newRoute')}
            </Button>
          ) : undefined
        }
      />
      <RequireSite>
        {(site) => (
          <>
            <Tabs idBase="pat" label={t('pat.title')} tabs={tabs} active={tab} onChange={setTab} />
            <TabPanel idBase="pat" active={tab} className="pt-5">
              {tab === 'live' && <LiveTab site={site} onShowHistory={() => setTab('sessions')} />}
              {tab === 'sessions' && <SessionsTab site={site} />}
              {tab === 'routes' && <RoutesTab site={site} canEdit={canEdit} />}
            </TabPanel>
          </>
        )}
      </RequireSite>
    </div>
  )
}
