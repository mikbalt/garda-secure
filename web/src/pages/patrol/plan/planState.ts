import { planPos } from './floorplans'
import type { CheckpointProgress, SessionProgress } from '../../../types'

/** Marker state on the live plan (FR-PAT-05). */
export type PlanState = 'scanned' | 'late' | 'next' | 'overdue' | 'missed' | 'pending'

export function planState(cp: CheckpointProgress, index: number, nextIndex: number | undefined | null): PlanState {
  if (cp.status === 'on_time') return 'scanned'
  if (cp.status === 'late') return 'late'
  if (cp.status === 'overdue') return 'overdue'
  if (cp.status === 'missed') return 'missed'
  return index === nextIndex ? 'next' : 'pending'
}

/** Whether any checkpoint of the session is placed on a floor plan. */
export function hasPlanPositions(p: Pick<SessionProgress, 'checkpoints'> | undefined): boolean {
  return !!p?.checkpoints.some((c) => planPos(c))
}
