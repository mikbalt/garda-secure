import { RequireSite } from '../components/RequireSite'
import { PageHeader } from '../components/ui/States'
import { Tabs, type TabItem } from '../components/ui/Tabs'
import { useAuth } from '../hooks/useAuth'
import { useTabParam } from '../hooks/useTabParam'
import { useT } from '../i18n/context'
import { LiveTab } from './patrol/LiveTab'
import { SessionsTab } from './patrol/SessionsTab'
import { RoutesTab } from './patrol/RoutesTab'

type TabKey = 'live' | 'sessions' | 'routes'

export default function Patrol() {
  const t = useT()
  const { can } = useAuth()
  // Route definitions (and QR sheets) are for operations staff; clients see live + history.
  const showRoutes = can('manageIncidents')
  const tabs: TabItem<TabKey>[] = [
    { key: 'live', label: t('pat.tabLive') },
    { key: 'sessions', label: t('pat.tabSessions') },
    ...(showRoutes ? [{ key: 'routes' as const, label: t('pat.tabRoutes') }] : []),
  ]
  const [tab, setTab] = useTabParam<TabKey>(
    tabs.map((x) => x.key),
    'live'
  )
  return (
    <div>
      <PageHeader title={t('pat.title')} />
      <RequireSite>
        {(site) => (
          <>
            <Tabs label={t('pat.title')} tabs={tabs} active={tab} onChange={setTab} />
            <div className="pt-4">
              {tab === 'live' && <LiveTab site={site} />}
              {tab === 'sessions' && <SessionsTab site={site} />}
              {tab === 'routes' && <RoutesTab site={site} canEdit={can('editRoutes')} />}
            </div>
          </>
        )}
      </RequireSite>
    </div>
  )
}
