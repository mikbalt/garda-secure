import { RequireSite } from '../components/RequireSite'
import { PageHeader } from '../components/ui/States'
import { Tabs, type TabItem } from '../components/ui/Tabs'
import { useAuth } from '../hooks/useAuth'
import { useTabParam } from '../hooks/useTabParam'
import { useT } from '../i18n/context'
import type { Capability } from '../lib/roles'
import { DailyReport } from './reports/DailyReport'
import { MonthlyReport } from './reports/MonthlyReport'
import { PatrolReport } from './reports/PatrolReport'
import { IncidentReport } from './reports/IncidentReport'
import { KpiReport } from './reports/KpiReport'

type TabKey = 'daily' | 'monthly' | 'patrol' | 'incidents' | 'kpi'

const defs: { key: TabKey; cap: Capability }[] = [
  { key: 'daily', cap: 'reportAttendanceDaily' },
  { key: 'monthly', cap: 'reportAttendanceMonthly' },
  { key: 'patrol', cap: 'reportPatrol' },
  { key: 'incidents', cap: 'reportIncidents' },
  { key: 'kpi', cap: 'reportKpi' },
]

export default function Reports() {
  const t = useT()
  const { can } = useAuth()
  const labels: Record<TabKey, string> = {
    daily: t('rep.daily'),
    monthly: t('rep.monthly'),
    patrol: t('rep.patrol'),
    incidents: t('rep.incidents'),
    kpi: t('rep.kpi'),
  }
  const tabs: TabItem<TabKey>[] = defs.filter((d) => can(d.cap)).map((d) => ({ key: d.key, label: labels[d.key] }))
  const [tab, setTab] = useTabParam<TabKey>(
    tabs.map((x) => x.key),
    tabs[0]?.key ?? 'daily'
  )
  return (
    <div>
      <div className="no-print">
        <PageHeader title={t('rep.title')} />
      </div>
      <RequireSite>
        {(site) => (
          <>
            <Tabs label={t('rep.title')} tabs={tabs} active={tab} onChange={setTab} />
            <div className="pt-4">
              {tab === 'daily' && <DailyReport site={site} />}
              {tab === 'monthly' && <MonthlyReport site={site} />}
              {tab === 'patrol' && <PatrolReport site={site} />}
              {tab === 'incidents' && <IncidentReport site={site} />}
              {tab === 'kpi' && <KpiReport site={site} />}
            </div>
          </>
        )}
      </RequireSite>
    </div>
  )
}
