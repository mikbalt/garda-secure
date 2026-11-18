import type { ReactNode } from 'react'
import { avatarTone, initials } from '../../lib/avatar'

const avatarSizes = { sm: 'h-7 w-7 text-[11px]', md: 'h-9 w-9 text-xs', lg: 'h-11 w-11 text-sm' }

/** Round initials avatar with a stable colour per name. */
export function Avatar({
  name,
  size = 'md',
  muted = false,
  className,
}: {
  name?: string | null
  size?: keyof typeof avatarSizes
  muted?: boolean
  /** Override size / layout classes when the preset sizes don't fit. */
  className?: string
}) {
  return (
    <span
      aria-hidden="true"
      className={`inline-flex shrink-0 select-none items-center justify-center rounded-full font-semibold ring-1 ${
        className ?? avatarSizes[size]
      } ${muted ? 'bg-gray-100 text-gray-500 ring-gray-200' : avatarTone(name)}`}
    >
      {initials(name)}
    </span>
  )
}

/** Avatar + name + secondary line (employee id or other meta). */
export function PersonCell({
  name,
  sub,
  size = 'md',
  muted,
  extra,
}: {
  name: string
  sub?: ReactNode
  size?: keyof typeof avatarSizes
  muted?: boolean
  extra?: ReactNode
}) {
  return (
    <div className="flex min-w-0 items-center gap-3">
      <Avatar name={name} size={size} muted={muted} />
      <div className="min-w-0">
        <div className="flex min-w-0 items-center gap-2">
          <span className={`truncate font-medium ${muted ? 'text-gray-500' : 'text-gray-900'}`}>{name}</span>
          {extra}
        </div>
        {sub && <div className="truncate text-xs text-gray-500">{sub}</div>}
      </div>
    </div>
  )
}
