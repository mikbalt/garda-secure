import { PageHeader } from '../components/ui/States'
import { Tabs, type TabItem } from '../components/ui/Tabs'
import { useTabParam } from '../hooks/useTabParam'
import { useT } from '../i18n/context'
import { SitesTab } from './settings/SitesTab'
import { DevicesTab } from './settings/DevicesTab'
import { RulesTab } from './settings/RulesTab'
import { ChannelsTab } from './settings/ChannelsTab'
import { LogTab } from './settings/LogTab'
import { BackupsTab } from './settings/BackupsTab'

type TabKey = 'sites' | 'devices' | 'rules' | 'channels' | 'log' | 'backups'
const KEYS: TabKey[] = ['sites', 'devices', 'rules', 'channels', 'log', 'backups']

export default function Settings() {
  const t = useT()
  const [tab, setTab] = useTabParam<TabKey>(KEYS, 'sites')
  const tabs: TabItem<TabKey>[] = [
    { key: 'sites', label: t('set.tabSites') },
    { key: 'devices', label: t('set.tabDevices') },
    { key: 'rules', label: t('set.tabRules') },
    { key: 'channels', label: t('set.tabChannels') },
    { key: 'log', label: t('set.tabLog') },
    { key: 'backups', label: t('set.tabBackups') },
  ]
  return (
    <div>
      <PageHeader title={t('set.title')} />
      <Tabs label={t('set.title')} tabs={tabs} active={tab} onChange={setTab} />
      <div className="pt-4">
        {tab === 'sites' && <SitesTab />}
        {tab === 'devices' && <DevicesTab />}
        {tab === 'rules' && <RulesTab />}
        {tab === 'channels' && <ChannelsTab />}
        {tab === 'log' && <LogTab />}
        {tab === 'backups' && <BackupsTab />}
      </div>
    </div>
  )
}
