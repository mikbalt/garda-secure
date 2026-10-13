import { useState, type ReactNode } from 'react'
import { Activity, CheckCircle2, CircleAlert, ClipboardCopy, HelpCircle, TriangleAlert } from 'lucide-react'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Modal } from '../../components/ui/Modal'
import { useI18n, type Key } from '../../i18n/context'
import { formatDateTime } from '../../lib/time'
import type { Device, DeviceAccount } from '../../types'
import { CLOCK_SKEW_MS, deviceHealth, formatOffset, type HealthLevel } from './deviceHealth'
import { relativeTime } from './format'

const LEVEL_TONE = { ok: 'green', warn: 'yellow', critical: 'red', unknown: 'gray' } as const

/** Compact health badge for the devices list; clicking opens the details. */
export function HealthBadge({ device, onOpen, now = Date.now() }: { device: Device; onOpen: () => void; now?: number }) {
  const { t } = useI18n()
  const r = deviceHealth(device, now)
  const Icon = r.level === 'ok' ? CheckCircle2 : r.level === 'unknown' ? HelpCircle : r.level === 'critical' ? CircleAlert : TriangleAlert
  const label =
    r.level === 'ok' ? t('acc.health.ok') : r.level === 'unknown' ? t('acc.health.unknown') : t('acc.health.issues', { n: r.issues.length })
  const tone = { ok: 'text-emerald-700 ring-emerald-200 bg-emerald-50', warn: 'text-amber-800 ring-amber-200 bg-amber-50', critical: 'text-red-700 ring-red-200 bg-red-50', unknown: 'text-gray-700 ring-gray-200 bg-gray-100' }[r.level]
  return (
    <button
      type="button"
      onClick={onOpen}
      aria-label={`${t('acc.health.title')}: ${label} — ${device.name}`}
      className={`inline-flex items-center gap-1.5 whitespace-nowrap rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset transition-colors hover:brightness-95 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 ${tone}`}
    >
      <Icon className="h-3 w-3" aria-hidden="true" />
      {label}
    </button>
  )
}

function Row({ label, children, warn }: { label: string; children: ReactNode; warn?: boolean }) {
  return (
    <div className="grid grid-cols-[minmax(0,11rem)_minmax(0,1fr)] gap-3 px-4 py-2.5 text-sm">
      <dt className="text-gray-600">{label}</dt>
      <dd className={`min-w-0 break-words ${warn ? 'font-medium text-amber-800' : 'text-gray-900'}`}>{children}</dd>
    </div>
  )
}

/** Details of the latest health beacon of a tablet, with explained warnings. */
export function DeviceHealthDrawer({
  device,
  siteName,
  tz,
  account,
  onClose,
}: {
  device: Device
  siteName: string
  tz: string
  account?: DeviceAccount
  onClose: () => void
}) {
  const { t, lang } = useI18n()
  const now = Date.now()
  const [copied, setCopied] = useState<boolean | null>(null)
  // Raw beacon for support tickets (no secrets in it: versions, counters, error texts).
  const copyDiagnostics = async () => {
    const text = JSON.stringify({ device: { id: device.id, name: device.name, site_id: device.site_id, last_seen_at: device.last_seen_at }, health_at: device.health_at, health: device.health ?? null }, null, 2)
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
    } catch {
      setCopied(false)
    }
  }
  const report = deviceHealth(device, now)
  const h = device.health
  const when = (iso?: string | null) => (iso ? `${relativeTime(iso, lang, now)} · ${formatDateTime(iso, tz, lang)}` : '—')
  const off = h ? formatOffset(h.clock_offset_ms ?? 0) : null
  const yes = (b?: boolean) => (b ? t('common.yes') : t('common.no'))
  const levelLabel: Record<HealthLevel, string> = {
    ok: t('acc.health.ok'),
    warn: t('acc.health.levelWarn'),
    critical: t('acc.health.levelCritical'),
    unknown: t('acc.health.unknown'),
  }

  return (
    <Modal open variant="drawer" onClose={onClose} title={t('acc.health.drawerTitle', { name: device.name })} footer={<Button variant="secondary" onClick={onClose}>{t('common.close')}</Button>}>
      <div className="space-y-5">
        <div className="flex flex-wrap items-center gap-2 text-sm text-gray-600">
          <Activity className="h-4 w-4 text-gray-500" aria-hidden="true" />
          <span>{siteName}</span>
          <Badge tone={LEVEL_TONE[report.level]} dot>
            {levelLabel[report.level]}
          </Badge>
          {account && <span className="text-xs text-gray-600">{t('acc.health.account', { username: account.username })}</span>}
        </div>

        {report.issues.length > 0 ? (
          <ul className="space-y-2">
            {report.issues.map((i) => (
              <li
                key={i.key}
                className={`flex items-start gap-2.5 rounded-xl border px-3.5 py-2.5 text-sm ${
                  i.level === 'critical' ? 'border-red-200 bg-red-50 text-red-900' : 'border-amber-200 bg-amber-50 text-amber-900'
                }`}
              >
                {i.level === 'critical' ? (
                  <CircleAlert className="mt-0.5 h-4 w-4 shrink-0 text-red-600" aria-hidden="true" />
                ) : (
                  <TriangleAlert className="mt-0.5 h-4 w-4 shrink-0 text-amber-600" aria-hidden="true" />
                )}
                <div>
                  <p className="font-semibold">{t(`acc.health.issue.${i.key}` as Key)}</p>
                  <p className="mt-0.5">{t(`acc.health.fix.${i.key}` as Key)}</p>
                </div>
              </li>
            ))}
          </ul>
        ) : (
          <div className="flex items-start gap-2.5 rounded-xl border border-emerald-200 bg-emerald-50 px-3.5 py-2.5 text-sm text-emerald-900">
            <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-emerald-600" aria-hidden="true" />
            <span>{t('acc.health.allGood')}</span>
          </div>
        )}

        {h ? (
          <dl className="divide-y divide-gray-100 overflow-hidden rounded-xl border border-gray-200">
            <Row label={t('acc.health.beacon')} warn={report.issues.some((i) => i.key === 'stale')}>
              {when(device.health_at ?? h.received_at)}
            </Row>
            <Row label={t('acc.health.app')}>
              {h.app_version}
              {h.app_build ? ` (${h.app_build})` : ''}
              {h.platform ? ` · ${h.platform}${h.os_version ? ` ${h.os_version}` : ''}` : ''}
            </Row>
            <Row label={t('acc.health.lastSync')}>{when(h.last_sync)}</Row>
            <Row label={t('acc.health.roster')} warn={report.issues.some((i) => i.key === 'rosterOld' || i.key === 'rosterBlocked')}>
              {when(h.roster_synced_at)}
            </Row>
            <Row label={t('acc.health.outbox')} warn={report.issues.some((i) => i.key.startsWith('outbox'))}>
              {t('acc.health.outboxValue', { pending: h.outbox_pending, failed: h.outbox_failed })}
              {h.outbox_paused ? ` · ${t('acc.health.paused')}` : ''}
            </Row>
            <Row label={t('acc.health.sos')} warn={h.sos_pending > 0}>
              {h.sos_pending}
            </Row>
            <Row label={t('acc.health.clock')} warn={Math.abs(h.clock_offset_ms ?? 0) > CLOCK_SKEW_MS}>
              {off && off.sign === '' ? t('acc.health.clockExact') : off && t('acc.health.clockValue', { sign: off.sign, m: off.minutes, s: off.seconds })}
            </Row>
            <Row label={t('acc.health.privateCa')}>{yes(h.private_ca)}</Row>
            {h.pin_failures !== undefined && <Row label={t('acc.health.pinFailures')}>{h.pin_failures}</Row>}
            {h.db_key_reset !== undefined && <Row label={t('acc.health.dbKeyReset')} warn={!!h.db_key_reset}>{yes(h.db_key_reset)}</Row>}
          </dl>
        ) : (
          <p className="text-sm text-gray-600">{t('acc.health.noBeaconBody')}</p>
        )}

        <div className="flex flex-wrap items-center gap-3">
          <Button size="sm" variant="secondary" onClick={copyDiagnostics}>
            <ClipboardCopy className="h-3.5 w-3.5" aria-hidden="true" />
            {t('acc.health.copy')}
          </Button>
          <p role="status" className="text-xs text-gray-600">
            {copied === true ? t('acc.secret.copied') : copied === false ? t('acc.secret.copyFailed') : ''}
          </p>
        </div>

        {h?.errors && h.errors.length > 0 && (
          <div>
            <h3 className="mb-2 text-sm font-semibold text-gray-900">{t('acc.health.errors')}</h3>
            <ul className="space-y-1.5">
              {h.errors.map((e, i) => (
                <li key={i} className="break-words rounded-lg bg-gray-50 px-3 py-2 font-mono text-xs text-gray-800 ring-1 ring-inset ring-gray-200">
                  {e}
                </li>
              ))}
            </ul>
          </div>
        )}
      </div>
    </Modal>
  )
}
