import type { ReactNode } from 'react'
import { ShieldOff } from 'lucide-react'
import { useAuth } from '../hooks/useAuth'
import { useT } from '../i18n/context'
import type { Capability } from '../lib/roles'

interface ProtectedRouteProps {
  cap: Capability
  children: ReactNode
}

/** Renders children only when the user's roles grant the capability. */
export function ProtectedRoute({ cap, children }: ProtectedRouteProps) {
  const { can } = useAuth()
  const t = useT()
  if (!can(cap)) {
    return (
      <div className="flex h-full flex-col items-center justify-center py-24 text-center">
        <ShieldOff className="mb-4 h-12 w-12 text-gray-500" aria-hidden="true" />
        <h1 className="mb-2 text-2xl font-semibold text-gray-900">{t('access.deniedTitle')}</h1>
        <p className="text-gray-700">{t('access.deniedBody')}</p>
      </div>
    )
  }
  return <>{children}</>
}
