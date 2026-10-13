import { forwardRef, useState, type ReactNode } from 'react'
import { ImageOff } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'
import { Button } from '../../../components/ui/Button'
import { useT } from '../../../i18n/context'
import { mediaUrl } from '../../../lib/media'
import { floorplansKey } from './floorplans'
import type { Floorplan } from '../../../types'

interface Props {
  plan: Floorplan
  /** Text alternative of the picture (the markers are mirrored as text elsewhere). */
  alt: string
  /** Overlay content, positioned in % of the image (markers, path). */
  children?: ReactNode
  className?: string
  /** Crosshair cursor and pointer handlers for placing checkpoints. */
  interactive?: boolean
  onPointerDown?: (e: React.PointerEvent<HTMLDivElement>) => void
}

/**
 * A floor-plan image with an overlay layer sized exactly like the picture
 * (the container keeps the plan's aspect ratio), so overlay children can be
 * placed with `left/top` percentages = relative plan coordinates (D-048).
 */
export const PlanImage = forwardRef<HTMLDivElement, Props>(function PlanImage({ plan, alt, children, className = '', interactive, onPointerDown }, ref) {
  const t = useT()
  const qc = useQueryClient()
  const [failed, setFailed] = useState(false)
  const src = mediaUrl(plan.url)
  const ratio = plan.width > 0 && plan.height > 0 ? `${plan.width} / ${plan.height}` : '3 / 2'

  return (
    <div className={`overflow-hidden rounded-xl border border-gray-200 bg-gray-50 ${className}`}>
      <div
        ref={ref}
        className={`relative w-full select-none ${interactive ? 'cursor-crosshair touch-none' : ''}`}
        style={{ aspectRatio: ratio }}
        onPointerDown={onPointerDown}
      >
        {src && !failed ? (
          <img src={src} alt={alt} draggable={false} className="pointer-events-none absolute inset-0 h-full w-full object-fill" onError={() => setFailed(true)} />
        ) : (
          <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 p-4 text-center text-sm text-gray-600">
            <ImageOff className="h-6 w-6 text-gray-500" aria-hidden="true" />
            <p>{t('p2.fp.imageFailed')}</p>
            <Button
              size="sm"
              variant="secondary"
              onClick={(e) => {
                e.stopPropagation()
                setFailed(false)
                void qc.invalidateQueries({ queryKey: floorplansKey(plan.site_id) })
              }}
              onPointerDown={(e) => e.stopPropagation()}
            >
              {t('app.retry')}
            </Button>
          </div>
        )}
        {children}
      </div>
    </div>
  )
})

/** Route path between positioned points (relative 0..1), in order. */
export function PlanPath({ points, dashedFrom }: { points: { x: number; y: number }[]; /** segments from this index on are drawn dashed (not walked yet) */ dashedFrom?: number }) {
  if (points.length < 2) return null
  const solid = dashedFrom === undefined ? points : points.slice(0, Math.max(1, dashedFrom + 1))
  const dashed = dashedFrom === undefined ? [] : points.slice(Math.max(0, dashedFrom))
  const toAttr = (ps: { x: number; y: number }[]) => ps.map((p) => `${(p.x * 100).toFixed(2)},${(p.y * 100).toFixed(2)}`).join(' ')
  return (
    <svg className="pointer-events-none absolute inset-0 h-full w-full" viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true">
      {solid.length >= 2 && <polyline points={toAttr(solid)} fill="none" className="stroke-primary-600" strokeWidth={3} strokeLinejoin="round" strokeLinecap="round" vectorEffect="non-scaling-stroke" opacity={0.75} />}
      {dashed.length >= 2 && (
        <polyline
          points={toAttr(dashed)}
          fill="none"
          className="stroke-gray-500"
          strokeWidth={2.5}
          strokeDasharray="6 6"
          strokeLinejoin="round"
          strokeLinecap="round"
          vectorEffect="non-scaling-stroke"
          opacity={0.8}
        />
      )}
    </svg>
  )
}
