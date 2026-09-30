import { getUserRoles } from '../keycloak'

interface ProtectedRouteProps {
  roles: string[]
  children: React.ReactNode
}

/**
 * Renders children only if the authenticated user has at least one of the required roles.
 * Shows a 403 message otherwise.
 */
export function ProtectedRoute({ roles, children }: ProtectedRouteProps) {
  const userRoles = getUserRoles()
  const hasAccess = roles.some((r) => userRoles.includes(r))

  if (!hasAccess) {
    return (
      <div className="flex flex-col items-center justify-center h-full py-24 text-center">
        <div className="text-6xl mb-4">🚫</div>
        <h2 className="text-2xl font-semibold text-gray-800 mb-2">Access Denied</h2>
        <p className="text-gray-500">
          You don't have permission to view this page.
        </p>
      </div>
    )
  }

  return <>{children}</>
}
