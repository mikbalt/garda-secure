import { useState } from 'react'
import { CalendarDays, CalendarPlus, Plus } from 'lucide-react'
import { RequireSite } from '../components/RequireSite'
import { Button } from '../components/ui/Button'
import { Notice, PageHeader } from '../components/ui/States'
import { TabPanel, Tabs } from '../components/ui/Tabs'
import { useAuth } from '../hooks/useAuth'
import { useNewParam } from '../hooks/useNewParam'
import { useTabParam } from '../hooks/useTabParam'
import { useT } from '../i18n/context'
import { todayIn } from '../lib/time'
import { RosterTab } from './schedules/RosterTab'
import { ShiftModal } from './schedules/ShiftModal'
import { TemplateModal } from './schedules/TemplateModal'
import { TemplatesTab } from './schedules/TemplatesTab'
import type { Shift, ShiftTemplate, Site } from '../types'

type TabKey = 'roster' | 'templates'

interface ShiftDraft {
  shift: Shift | null
  guardId?: string
  date: string
}

function SchedulesContent({ site }: { site: Site }) {
  const t = useT()
  const { can } = useAuth()
  const canEdit = can('editSchedules')
  const [tab, setTab] = useTabParam<TabKey>(['roster', 'templates'], 'roster')
  const [pattern, setPattern] = useState<ShiftTemplate | 'new' | null>(null)
  const [shift, setShift] = useState<ShiftDraft | null>(null)

  // The setup guide links to /schedules?tab=templates&new=1.
  useNewParam(() => setPattern('new'), canEdit)

  const newPattern = () => setPattern('new')

  return (
    <>
      <PageHeader
        icon={CalendarDays}
        title={t('sch.title')}
        subtitle={t('ppl.sch.subtitle')}
        actions={
          canEdit ? (
            <>
              <Button variant="secondary" onClick={() => setShift({ shift: null, date: todayIn(site.timezone) })}>
                <CalendarPlus className="h-4 w-4" aria-hidden="true" />
                {t('ppl.sch.addOneOff')}
              </Button>
              <Button onClick={newPattern}>
                <Plus className="h-4 w-4" aria-hidden="true" />
                {t('ppl.sch.newPattern')}
              </Button>
            </>
          ) : undefined
        }
      />
      {!canEdit && <Notice>{t('ppl.sch.readOnly')}</Notice>}
      <div className="mb-4">
        <Tabs
          idBase="sch"
          label={t('sch.title')}
          active={tab}
          onChange={setTab}
          tabs={[
            { key: 'roster', label: t('ppl.sch.tabRoster') },
            { key: 'templates', label: t('ppl.sch.tabPatterns') },
          ]}
        />
      </div>
      <TabPanel idBase="sch" active={tab}>
      {tab === 'roster' ? (
        <RosterTab
          site={site}
          canEdit={canEdit}
          onAdd={(guardId, date) => setShift({ shift: null, guardId, date })}
          onEdit={(s) => setShift({ shift: s, date: s.start_time })}
          onNewPattern={newPattern}
        />
      ) : (
        <TemplatesTab site={site} canEdit={canEdit} onNew={newPattern} onEdit={setPattern} />
      )}
      </TabPanel>

      {pattern && <TemplateModal site={site} template={pattern === 'new' ? null : pattern} onClose={() => setPattern(null)} />}
      {shift && (
        <ShiftModal site={site} shift={shift.shift} defaultDate={shift.date} defaultGuardId={shift.guardId} onClose={() => setShift(null)} />
      )}
    </>
  )
}

export default function Schedules() {
  return <RequireSite>{(site) => <SchedulesContent site={site} />}</RequireSite>
}
