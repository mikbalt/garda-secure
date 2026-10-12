import { useEffect, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import { BellRing, Building2, DatabaseBackup, History, Send, ServerCog, Settings2, Siren, Tablet, type LucideIcon } from 'lucide-react'
import { apiGet } from '../api/client'
import { PageHeader } from '../components/ui/States'
import { useAuth } from '../hooks/useAuth'
import { useTabParam } from '../hooks/useTabParam'
import { useI18n, type Key } from '../i18n/context'
import { SitesTab } from './settings/SitesTab'
import { DevicesTab } from './settings/DevicesTab'
import { RulesTab } from './settings/RulesTab'
import { ChannelsTab } from './settings/ChannelsTab'
import { LogTab } from './settings/LogTab'
import { BackupsTab } from './settings/BackupsTab'
import { OpsRulesTab } from './settings/OpsRulesTab'
import { PlatformInfo } from './settings/PlatformInfo'
import type { Device, NotificationLogEntry } from '../types'

type TabKey = 'sites' | 'devices' | 'rules' | 'log' | 'channels' | 'ops' | 'opslog' | 'backups' | 'platform'

/** Who sees a section: realm admin, platform-admin, or a realm admin without platform rights. */
type Audience = 'realm' | 'platform' | 'realmOnly'

interface Section {
  key: TabKey
  icon: LucideIcon
  label: Key
  desc: Key
  group: Key
  audience: Audience
}

const SECTIONS: Section[] = [
  { key: 'sites', icon: Building2, label: 'adm.set.sites', desc: 'adm.set.sitesDesc', group: 'adm.set.groupSetup', audience: 'realm' },
  { key: 'devices', icon: Tablet, label: 'adm.set.devices', desc: 'adm.set.devicesDesc', group: 'adm.set.groupSetup', audience: 'realm' },
  { key: 'rules', icon: BellRing, label: 'adm.set.rules', desc: 'adm.set.rulesDesc', group: 'adm.set.groupNotif', audience: 'realm' },
  { key: 'log', icon: History, label: 'adm.set.log', desc: 'adm.set.logDesc', group: 'adm.set.groupNotif', audience: 'realm' },
  // Install-wide (D-025/D-032): platform-admin only.
  { key: 'channels', icon: Send, label: 'adm.set.channels', desc: 'adm.set.channelsDesc', group: 'plat.group', audience: 'platform' },
  { key: 'ops', icon: Siren, label: 'plat.opsRules', desc: 'plat.opsRulesDesc', group: 'plat.group', audience: 'platform' },
  { key: 'opslog', icon: History, label: 'plat.opsLog', desc: 'plat.opsLogDesc', group: 'plat.group', audience: 'platform' },
  { key: 'backups', icon: DatabaseBackup, label: 'adm.set.backups', desc: 'adm.set.backupsDesc', group: 'plat.group', audience: 'platform' },
  { key: 'platform', icon: ServerCog, label: 'plat.infoTitle', desc: 'plat.infoDesc', group: 'plat.group', audience: 'realmOnly' },
]

function CountBadge({ n, tone }: { n: number; tone: 'amber' | 'red' }) {
  if (!n) return null
  return (
    <span
      className={`min-w-[1.25rem] rounded-full px-1.5 py-px text-center text-[11px] font-semibold tabular-nums ${
        tone === 'amber' ? 'bg-amber-100 text-amber-800' : 'bg-red-100 text-red-700'
      }`}
    >
      {n}
    </span>
  )
}

function SettingsNav({
  sections,
  active,
  onChange,
  badges,
}: {
  sections: Section[]
  active: TabKey
  onChange: (k: TabKey) => void
  badges: Partial<Record<TabKey, { n: number; tone: 'amber' | 'red' }>>
}) {
  const { t } = useI18n()
  const groups = Array.from(new Set(sections.map((s) => s.group)))
  const pills = useRef<HTMLUListElement>(null)
  // Keep the selected pill visible on small screens.
  useEffect(() => {
    const ul = pills.current
    const el = ul?.querySelector<HTMLElement>('[aria-current="page"]')
    if (ul && el) ul.scrollLeft = el.parentElement!.offsetLeft - ul.offsetLeft - 16
  }, [active])
  return (
    <nav aria-label={t('set.title')} className="no-print">
      {/* Mobile / tablet: scrollable pills */}
      <ul ref={pills} className="-mx-4 mb-5 flex gap-2 overflow-x-auto px-4 pb-1 [scrollbar-width:none] sm:mx-0 sm:px-0 lg:hidden [&::-webkit-scrollbar]:hidden">
        {sections.map((s) => {
          const selected = s.key === active
          const b = badges[s.key]
          return (
            <li key={s.key} className="shrink-0">
              <button
                type="button"
                aria-current={selected ? 'page' : undefined}
                onClick={() => onChange(s.key)}
                className={`inline-flex h-9 items-center gap-2 rounded-full px-3.5 text-sm font-medium transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 ${
                  selected ? 'bg-ink-900 text-white shadow-sm' : 'bg-white text-gray-700 ring-1 ring-inset ring-gray-200 hover:bg-gray-50'
                }`}
              >
                <s.icon className="h-4 w-4" aria-hidden="true" />
                {t(s.label)}
                {b && <CountBadge n={b.n} tone={b.tone} />}
              </button>
            </li>
          )
        })}
      </ul>

      {/* Desktop: vertical list with descriptions */}
      <div className="sticky top-6 hidden space-y-5 lg:block">
        {groups.map((g) => (
          <div key={g}>
            <p className="mb-1.5 px-3 text-[11px] font-semibold uppercase tracking-wider text-gray-500">{t(g)}</p>
            <ul className="space-y-1">
              {sections.filter((s) => s.group === g).map((s) => {
                const selected = s.key === active
                const b = badges[s.key]
                return (
                  <li key={s.key}>
                    <button
                      type="button"
                      aria-current={selected ? 'page' : undefined}
                      onClick={() => onChange(s.key)}
                      className={`group flex w-full items-start gap-3 rounded-xl p-3 text-left transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 ${
                        selected ? 'bg-white shadow-card ring-1 ring-gray-200/80' : 'hover:bg-white/70'
                      }`}
                    >
                      <span
                        className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-lg transition-colors ${
                          selected ? 'bg-primary-50 text-primary-600 ring-1 ring-primary-100' : 'bg-gray-100 text-gray-500 group-hover:text-gray-700'
                        }`}
                      >
                        <s.icon className="h-[18px] w-[18px]" aria-hidden="true" />
                      </span>
                      <span className="min-w-0 flex-1">
                        <span className="flex items-center justify-between gap-2">
                          <span className={`text-sm font-medium ${selected ? 'text-gray-900' : 'text-gray-700'}`}>{t(s.label)}</span>
                          {b && <CountBadge n={b.n} tone={b.tone} />}
                        </span>
                        <span className="mt-0.5 block text-xs leading-snug text-gray-600">{t(s.desc)}</span>
                      </span>
                    </button>
                  </li>
                )
              })}
            </ul>
          </div>
        ))}
      </div>
    </nav>
  )
}

export default function Settings() {
  const { t } = useI18n()
  const { can } = useAuth()
  const realm = can('manageRealmSettings')
  const platform = can('managePlatform')
  const sections = SECTIONS.filter((s) => (s.audience === 'realm' ? realm : s.audience === 'platform' ? platform : realm && !platform))
  const [tab, setTab] = useTabParam<TabKey>(
    sections.map((s) => s.key),
    sections[0]?.key ?? 'sites'
  )

  const devices = useQuery({ queryKey: ['devices'], queryFn: () => apiGet<Device[]>('/devices'), refetchInterval: 60_000, enabled: realm })
  const failed = useQuery({
    queryKey: ['notification-log', 'failed-count'],
    queryFn: () => apiGet<NotificationLogEntry[]>('/notifications/log', { status: 'failed', limit: 200 }),
    refetchInterval: 60_000,
    enabled: realm,
  })
  const opsFailed = useQuery({
    queryKey: ['ops-log', 'failed-count'],
    queryFn: () => apiGet<NotificationLogEntry[]>('/notifications/ops-log', { status: 'failed', limit: 200 }),
    refetchInterval: 60_000,
    enabled: platform,
  })
  const badges: Partial<Record<TabKey, { n: number; tone: 'amber' | 'red' }>> = {
    devices: { n: (devices.data ?? []).filter((d) => d.status === 'pending').length, tone: 'amber' },
    log: { n: (failed.data ?? []).filter((l) => l.status === 'failed').length, tone: 'red' },
    opslog: { n: (opsFailed.data ?? []).filter((l) => l.status === 'failed').length, tone: 'red' },
  }

  return (
    <div>
      <PageHeader icon={Settings2} title={t('set.title')} subtitle={t('adm.set.subtitle')} />
      <div className="lg:grid lg:grid-cols-[16.5rem_minmax(0,1fr)] lg:gap-8">
        <SettingsNav sections={sections} active={tab} onChange={setTab} badges={badges} />
        <div className="min-w-0">
          {tab === 'sites' && <SitesTab />}
          {tab === 'devices' && <DevicesTab />}
          {tab === 'rules' && <RulesTab />}
          {tab === 'log' && <LogTab />}
          {tab === 'channels' && <ChannelsTab />}
          {tab === 'ops' && <OpsRulesTab />}
          {tab === 'opslog' && <LogTab mode="ops" />}
          {tab === 'backups' && <BackupsTab />}
          {tab === 'platform' && <PlatformInfo />}
        </div>
      </div>
    </div>
  )
}
