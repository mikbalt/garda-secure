import { useQuery } from '@tanstack/react-query'
import { apiGet } from '../../../api/client'
import type { Checkpoint, Floorplan } from '../../../types'

/** Upload limits of POST /sites/:id/floorplans (docs/api.md). */
export const FLOORPLAN_MAX_BYTES = 10 * 1024 * 1024
export const FLOORPLAN_TYPES = ['image/jpeg', 'image/png', 'image/webp']
export const FLOORPLAN_MAX_PER_SITE = 20

export const floorplansKey = (siteId: string) => ['floorplans', siteId] as const

/**
 * Floor plans of a site. Image URLs are pre-signed for ~15 min, so the list
 * is refreshed every 10 min (and on demand when an image fails to load).
 */
export function useFloorplans(siteId: string, enabled = true) {
  return useQuery({
    queryKey: floorplansKey(siteId),
    queryFn: () => apiGet<Floorplan[]>(`/sites/${siteId}/floorplans`),
    enabled: enabled && !!siteId,
    staleTime: 5 * 60_000,
    refetchInterval: 10 * 60_000,
  })
}

/** Why a file cannot be uploaded as a floor plan (client-side check), or null. */
export function floorplanFileError(file: File | null | undefined): 'file_required' | 'invalid_file_type' | 'file_too_large' | null {
  if (!file) return 'file_required'
  if (!FLOORPLAN_TYPES.includes(file.type)) return 'invalid_file_type'
  if (file.size > FLOORPLAN_MAX_BYTES) return 'file_too_large'
  return null
}

export interface PlanPos {
  floorplan_id: string
  plan_x: number
  plan_y: number
}

/** A checkpoint's floor-plan position, when it has a complete one. */
export function planPos(cp: Pick<Checkpoint, 'floorplan_id' | 'plan_x' | 'plan_y'>): PlanPos | null {
  if (!cp.floorplan_id || cp.plan_x == null || cp.plan_y == null) return null
  return { floorplan_id: cp.floorplan_id, plan_x: cp.plan_x, plan_y: cp.plan_y }
}

export const clamp01 = (v: number) => Math.min(1, Math.max(0, v))

/** Rounds a relative coordinate to 4 decimals (0.01 % of the image). */
export const round4 = (v: number) => Math.round(clamp01(v) * 10_000) / 10_000

/** Percent text for a relative coordinate ("34.5"). */
export const pctText = (v: number) => String(Math.round(v * 1000) / 10)
