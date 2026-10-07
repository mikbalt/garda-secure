import { useQuery } from '@tanstack/react-query'
import { apiGet } from '../api/client'
import { RequireSite } from '../components/RequireSite'
import { PageHeader } from '../components/ui/States'
import { Tabs, type TabItem } from '../components/ui/Tabs'
import { useAuth } from '../hooks/useAuth'
import { useTabParam } from '../hooks/useTabParam'
import { useT } from '../i18n/context'
import { IncidentsTab } from './incidents/IncidentsTab'
import { SosTab } from './incidents/SosTab'
import { DefectsTab } from './incidents/DefectsTab'
import { VisitorsTab } from './incidents/VisitorsTab'
import type { SosAlert, Site } from '../types'

type TabKey = 'incidents' | 'sos' | 'defects' | 'visitors'

function IncidentsContent({ site }: { site: Site }) {
  const t = useT()
  const { can } = useAuth()
  const ops = can('viewSos')
  // Shares the cache entry polled by the SOS banner.
  const unack = useQuery({
    queryKey: ['sos', site.id, 'unack'],
    queryFn: () => apiGet<SosAlert[]>('/sos', { site_id: site.id, unack_only: true }),
    enabled: ops,
  })
  const tabs: TabItem<TabKey>[] = [
    { key: 'incidents', label: t('inc.tabIncidents') },
    ...(ops
      ? ([
          { key: 'sos', label: t('inc.tabSos'), badge: unack.data?.length },
          { key: 'defects', label: t('inc.tabDefects') },
          { key: 'visitors', label: t('inc.tabVisitors') },
        ] as TabItem<TabKey>[])
      : []),
  ]
  const [tab, setTab] = useTabParam<TabKey>(
    tabs.map((x) => x.key),
    'incidents'
  )
  return (
    <>
      {tabs.length > 1 && <Tabs label={t('inc.title')} tabs={tabs} active={tab} onChange={setTab} />}
      <div className="pt-4">
        {tab === 'incidents' && <IncidentsTab site={site} />}
        {tab === 'sos' && <SosTab site={site} />}
        {tab === 'defects' && <DefectsTab site={site} />}
        {tab === 'visitors' && <VisitorsTab site={site} />}
      </div>
    </>
  )
}

export default function Incidents() {
  const t = useT()
  return (
    <div>
      <PageHeader title={t('inc.title')} />
      <RequireSite>{(site) => <IncidentsContent site={site} />}</RequireSite>
    </div>
  )
}
