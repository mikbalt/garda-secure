import type { Severity, WorkflowStatus } from '../../types'

export const SEVERITIES: Severity[] = ['low', 'medium', 'high', 'critical']
export const WORKFLOW: WorkflowStatus[] = ['open', 'in_progress', 'resolved']

/** Categories the tablet offers (translated via `incType.*`). */
export const KNOWN_TYPES = ['theft', 'trespassing', 'suspicious', 'vandalism', 'fire_hazard', 'medical', 'accident', 'other']

/** Left severity bar colour. */
export const sevBar: Record<Severity, string> = {
  low: 'bg-gray-300',
  medium: 'bg-amber-400',
  high: 'bg-orange-500',
  critical: 'bg-red-600',
}
