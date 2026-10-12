import { DatabaseBackup, Send, ServerCog, Siren } from 'lucide-react'
import { useI18n } from '../../i18n/context'
import { IconTile, SectionHeader } from './kit'

/**
 * Shown to a realm admin without `platform-admin`: channel settings,
 * backups and ops alerts cover every client of the installation, so only
 * the platform operator manages them (D-025, D-032).
 */
export function PlatformInfo() {
  const { t } = useI18n()
  const items = [
    { icon: Send, title: t('adm.set.channels'), body: t('plat.infoChannels') },
    { icon: DatabaseBackup, title: t('adm.set.backups'), body: t('plat.infoBackups') },
    { icon: Siren, title: t('plat.opsRules'), body: t('plat.infoOps') },
  ]
  return (
    <>
      <SectionHeader title={t('plat.infoTitle')} description={t('plat.infoIntro')} />
      <div className="rounded-2xl border border-gray-200/80 bg-white p-5 shadow-card">
        <div className="flex items-start gap-3.5">
          <IconTile icon={ServerCog} tone="gray" />
          <div className="min-w-0">
            <p className="text-sm font-semibold text-gray-900">{t('plat.infoWho')}</p>
            <p className="mt-0.5 text-sm text-gray-700">{t('plat.infoWhoBody')}</p>
          </div>
        </div>
        <ul className="mt-5 space-y-3">
          {items.map((it) => (
            <li key={it.title} className="flex items-start gap-3 rounded-xl bg-gray-50 px-3.5 py-3 ring-1 ring-inset ring-gray-200/70">
              <it.icon className="mt-0.5 h-4 w-4 shrink-0 text-gray-600" aria-hidden="true" />
              <div className="min-w-0">
                <p className="text-sm font-medium text-gray-900">{it.title}</p>
                <p className="text-sm text-gray-700">{it.body}</p>
              </div>
            </li>
          ))}
        </ul>
      </div>
    </>
  )
}
