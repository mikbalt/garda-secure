import {
  CalendarRange,
  CalendarX2,
  FileWarning,
  Send,
  Clock4,
  DatabaseBackup,
  HardDriveDownload,
  Mail,
  MessageCircle,
  ServerCrash,
  ShieldAlert,
  Siren,
  TimerOff,
  UserX,
  Wrench,
  type LucideIcon,
} from 'lucide-react'
import type { Key } from '../../i18n/context'
import type { Channel, ChannelSettings } from '../../types'
import type { TintTone } from './kit'

export const CHANNEL_ICON: Record<Channel, LucideIcon> = { email: Mail, whatsapp: MessageCircle }
export const CHANNEL_TONE: Record<Channel, string> = { email: 'text-primary-600', whatsapp: 'text-emerald-600' }

/** Icon of a delivery channel; channels added later in the backend registry (D-047) get a generic one. */
export function channelIcon(ch: string): LucideIcon {
  return (CHANNEL_ICON as Record<string, LucideIcon>)[ch] ?? Send
}
export function channelTone(ch: string): string {
  return (CHANNEL_TONE as Record<string, string>)[ch] ?? 'text-gray-600'
}

export interface EventMeta {
  icon: LucideIcon
  tone: TintTone
  title: Key
  desc: Key
}

/** Display order and plain-language meaning of each notification event. */
export const EVENT_ORDER = [
  'sos',
  'incident_high',
  'incident_filed',
  'absent',
  'late_clock_in',
  'patrol_not_started',
  'sla_missed',
  'defect_opened',
  'monthly_recap',
  // install-wide ops alerts (platform-admin)
  'ops_sos_undelivered',
  'ops_backup_failed',
  'ops_backup_stale',
  'ops_job_failing',
]

export const EVENT_META: Record<string, EventMeta> = {
  sos: { icon: Siren, tone: 'red', title: 'adm.ev.sos', desc: 'adm.ev.sosDesc' },
  incident_high: { icon: ShieldAlert, tone: 'orange', title: 'adm.ev.incident', desc: 'adm.ev.incidentDesc' },
  incident_filed: { icon: FileWarning, tone: 'amber', title: 'p2.ev.incidentFiled', desc: 'p2.ev.incidentFiledDesc' },
  patrol_not_started: { icon: CalendarX2, tone: 'orange', title: 'p2.ev.notStarted', desc: 'p2.ev.notStartedDesc' },
  absent: { icon: UserX, tone: 'red', title: 'adm.ev.absent', desc: 'adm.ev.absentDesc' },
  late_clock_in: { icon: Clock4, tone: 'amber', title: 'adm.ev.late', desc: 'adm.ev.lateDesc' },
  sla_missed: { icon: TimerOff, tone: 'orange', title: 'adm.ev.sla', desc: 'adm.ev.slaDesc' },
  defect_opened: { icon: Wrench, tone: 'amber', title: 'adm.ev.defect', desc: 'adm.ev.defectDesc' },
  monthly_recap: { icon: CalendarRange, tone: 'blue', title: 'adm.ev.recap', desc: 'adm.ev.recapDesc' },
  ops_sos_undelivered: { icon: Siren, tone: 'red', title: 'plat.ev.sosUndelivered', desc: 'plat.ev.sosUndeliveredDesc' },
  ops_backup_failed: { icon: DatabaseBackup, tone: 'red', title: 'plat.ev.backupFailed', desc: 'plat.ev.backupFailedDesc' },
  ops_backup_stale: { icon: HardDriveDownload, tone: 'amber', title: 'plat.ev.backupStale', desc: 'plat.ev.backupStaleDesc' },
  ops_job_failing: { icon: ServerCrash, tone: 'orange', title: 'plat.ev.jobFailing', desc: 'plat.ev.jobFailingDesc' },
}

export function sortEvents(types: string[]): string[] {
  const rank = (e: string) => {
    const i = EVENT_ORDER.indexOf(e)
    return i < 0 ? EVENT_ORDER.length : i
  }
  return [...types].sort((a, b) => rank(a) - rank(b) || a.localeCompare(b))
}

const filled = (v: string | undefined) => !!v && v.trim() !== ''

/** Whether the install has enough settings to deliver on a channel. */
export function channelConfigured(s: ChannelSettings | undefined, ch: Channel): boolean {
  if (!s) return false
  if (ch === 'email') return filled(s.smtp?.host) && filled(s.smtp?.from)
  const w = s.whatsapp
  return filled(w?.api_url) && filled(w?.token) && filled(w?.phone_number_id) && filled(w?.template_name)
}

/** Shows WhatsApp numbers with a leading "+". */
export function displayRecipient(ch: string, r: string): string {
  return ch === 'whatsapp' && /^\d+$/.test(r) ? `+${r}` : r
}

export function validRecipient(ch: string, r: string): boolean {
  const v = r.trim()
  if (ch === 'email') return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v)
  if (ch === 'whatsapp') return /^\+?\d{8,15}$/.test(v.replace(/[\s-]/g, ''))
  // Unknown (newer) channel: the server validates the recipient.
  return v.length > 0 && v.length <= 300
}

/** Normalises a WhatsApp number to digits only (international format). */
export function normaliseRecipient(ch: string, r: string): string {
  const v = r.trim()
  if (ch !== 'whatsapp') return v
  let d = v.replace(/[^\d]/g, '')
  if (d.startsWith('0')) d = `62${d.slice(1)}`
  return d
}
