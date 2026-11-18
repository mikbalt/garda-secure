/**
 * Small building blocks shared by the Settings pages. General-purpose
 * components are promoted to components/ui and re-exported here for
 * backward compatibility.
 */
import type { ReactNode } from 'react'

// Re-exports from components/ui — import directly from there in new code.
export { Switch } from '../../components/ui/Switch'
export { ConfirmDialog } from '../../components/ui/ConfirmDialog'
export { IconTile, type TintTone } from '../../components/ui/IconTile'

/** Title + one-line purpose of a settings section, with its primary action. */
export function SectionHeader({ title, description, actions }: { title: string; description: ReactNode; actions?: ReactNode }) {
  return (
    <div className="mb-5 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
      <div className="min-w-0 sm:flex-1">
        <h2 className="text-lg font-semibold tracking-tight text-gray-900">{title}</h2>
        <p className="mt-0.5 max-w-2xl text-sm text-gray-500">{description}</p>
      </div>
      {actions && <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>}
    </div>
  )
}
