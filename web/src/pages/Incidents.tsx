import { useQuery } from '@tanstack/react-query'
import { ShieldAlert } from 'lucide-react'
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
import type { DashboardSummary, SosAlert, Site } from '../types'

type TabKey = 'incidents' | 'sos' | 'defects' | 'visitors'

function IncidentsContent({ site }: { site: Site }) {
  const t = useT()
  const { can } = useAuth()
  // Shares the cache entry polled by the SOS banner.
  const unack = useQuery({
    queryKey: ['sos', site.id, 'unack'],
    queryFn: () => apiGet<SosAlert[]>('/sos', { site_id: site.id, unack_only: true }),
    enabled: can('viewSos'),
  })
  // Shares the dashboard summary (open incidents / defects counts).
  const summary = useQuery({
    queryKey: ['dashboard', site.id],
    queryFn: () => apiGet<DashboardSummary>('/dashboard/summary', { site_id: site.id }),
    refetchInterval: 30_000,
  })
  const s = summary.data
  const tabs: TabItem<TabKey>[] = [
    { key: 'incidents', label: t('inc.tabIncidents'), badge: s?.open_incidents || undefined },
    ...(can('viewSos') ? [{ key: 'sos' as const, label: t('inc.tabSos'), badge: unack.data?.length || undefined }] : []),
    ...(can('viewDefects') ? [{ key: 'defects' as const, label: t('inc.tabDefects'), badge: s?.open_defects || undefined }] : []),
    ...(can('viewVisitors') ? [{ key: 'visitors' as const, label: t('inc.tabVisitors') }] : []),
  ]
  const [tab, setTab] = useTabParam<TabKey>(
    tabs.map((x) => x.key),
    'incidents'
  )
  return (
    <>
      {tabs.length > 1 && <Tabs label={t('inc.title')} tabs={tabs} active={tab} onChange={setTab} />}
      <div className={tabs.length > 1 ? 'pt-5' : ''}>
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
  const { can } = useAuth()
  return (
    <div>
      <PageHeader icon={ShieldAlert} title={t('inc.title')} subtitle={can('viewSos') ? t('ops.inc.subtitle') : t('ops.inc.subtitleRo')} />
      <RequireSite>{(site) => <IncidentsContent site={site} />}</RequireSite>
    </div>
  )
}
