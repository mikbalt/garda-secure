import {
  BellRing,
  CalendarX2,
  ClipboardCheck,
  Clock4,
  FileWarning,
  HeartPulse,
  ShieldAlert,
  Siren,
  TabletSmartphone,
  TimerOff,
  UserX,
  Wrench,
  type LucideIcon,
} from 'lucide-react'
import type { Key } from '../../i18n/context'
import type { TintTone } from '../settings/kit'

export interface PushGroup {
  key: string
  label: Key
  events: string[]
}

/** Push events grouped for the preference switches (order = display order). */
export const PUSH_GROUPS: PushGroup[] = [
  { key: 'sos', label: 'push.group.sos', events: ['sos', 'sos_unacknowledged', 'incident_high', 'incident_filed'] },
  { key: 'patrol', label: 'push.group.patrol', events: ['patrol_not_started', 'sla_missed'] },
  { key: 'attendance', label: 'push.group.attendance', events: ['absent', 'late_clock_in', 'attendance_review'] },
  { key: 'ops', label: 'push.group.ops', events: ['defect_opened', 'device_pending', 'device_unhealthy'] },
]

export const PUSH_EVENT_ICON: Record<string, { icon: LucideIcon; tone: TintTone }> = {
  sos: { icon: Siren, tone: 'red' },
  sos_unacknowledged: { icon: BellRing, tone: 'red' },
  incident_high: { icon: ShieldAlert, tone: 'orange' },
  incident_filed: { icon: FileWarning, tone: 'amber' },
  patrol_not_started: { icon: CalendarX2, tone: 'orange' },
  sla_missed: { icon: TimerOff, tone: 'orange' },
  absent: { icon: UserX, tone: 'red' },
  late_clock_in: { icon: Clock4, tone: 'amber' },
  attendance_review: { icon: ClipboardCheck, tone: 'blue' },
  defect_opened: { icon: Wrench, tone: 'amber' },
  device_pending: { icon: TabletSmartphone, tone: 'blue' },
  device_unhealthy: { icon: HeartPulse, tone: 'orange' },
}

/**
 * The groups with only the events the caller can receive (`available`);
 * events the backend adds later land in an "Other" group.
 */
export function groupAvailable(available: string[]): PushGroup[] {
  const set = new Set(available)
  const known = new Set(PUSH_GROUPS.flatMap((g) => g.events))
  const groups = PUSH_GROUPS.map((g) => ({ ...g, events: g.events.filter((e) => set.has(e)) })).filter((g) => g.events.length > 0)
  const other = available.filter((e) => !known.has(e))
  return other.length ? [...groups, { key: 'other', label: 'push.group.other', events: other }] : groups
}
