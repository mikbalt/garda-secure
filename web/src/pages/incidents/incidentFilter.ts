import { addDays, todayIn } from '../../lib/time'

export interface IncidentFilterValue {
  from: string
  to: string
  type: string
  severity: string
  status: string
}

/** Last 30 days, no other filters. */
export function defaultIncidentFilter(tz: string): IncidentFilterValue {
  const today = todayIn(tz)
  return { from: addDays(today, -29), to: today, type: '', severity: '', status: '' }
}
