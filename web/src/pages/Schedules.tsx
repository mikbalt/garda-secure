import { RequireSite } from '../components/RequireSite'
import { Notice, PageHeader } from '../components/ui/States'
import { Tabs } from '../components/ui/Tabs'
import { useAuth } from '../hooks/useAuth'
import { useTabParam } from '../hooks/useTabParam'
import { useT } from '../i18n/context'
import { ShiftsTab } from './schedules/ShiftsTab'
import { TemplatesTab } from './schedules/TemplatesTab'

type TabKey = 'shifts' | 'templates'

export default function Schedules() {
  const t = useT()
  const { can } = useAuth()
  const [tab, setTab] = useTabParam<TabKey>(['shifts', 'templates'], 'shifts')
  const canEdit = can('editSchedules')
  return (
    <div>
      <PageHeader title={t('sch.title')} />
      {!canEdit && <Notice>{t('sch.readOnly')}</Notice>}
      <RequireSite>
        {(site) => (
          <>
            <Tabs
              label={t('sch.title')}
              active={tab}
              onChange={setTab}
              tabs={[
                { key: 'shifts', label: t('sch.tabShifts') },
                { key: 'templates', label: t('sch.tabTemplates') },
              ]}
            />
            <div className="pt-4">
              {tab === 'shifts' ? <ShiftsTab site={site} canEdit={canEdit} /> : <TemplatesTab site={site} canEdit={canEdit} />}
            </div>
          </>
        )}
      </RequireSite>
    </div>
  )
}
