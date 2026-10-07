import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { Building2 } from 'lucide-react'
import { useSite } from '../context/site'
import { useAuth } from '../hooks/useAuth'
import { useT } from '../i18n/context'
import { TableSkeleton } from './LoadingSkeleton'
import { ErrorState } from './ui/States'
import type { Site } from '../types'

/**
 * Renders children with the selected site, or a loading / error / "no sites
 * yet" state (admins get a link to create the first site).
 */
export function RequireSite({ children }: { children: (site: Site) => ReactNode }) {
  const { site, isLoading, error } = useSite()
  const { can } = useAuth()
  const t = useT()
  if (isLoading) return <TableSkeleton />
  if (error) return <ErrorState error={error} />
  if (!site) {
    return (
      <div className="mx-auto mt-16 max-w-md rounded-xl border border-gray-200 bg-white p-8 text-center shadow-sm">
        <Building2 className="mx-auto mb-3 h-10 w-10 text-primary-700" aria-hidden="true" />
        <h2 className="text-lg font-semibold text-gray-900">{t('site.none')}</h2>
        <p className="mt-1 text-sm text-gray-700">{t('site.noneHint')}</p>
        {can('manageSites') ? (
          <Link
            to="/settings?tab=sites&new=1"
            className="mt-4 inline-flex rounded-md bg-primary-700 px-4 py-2 text-sm font-medium text-white hover:bg-primary-800 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-600 focus-visible:ring-offset-2"
          >
            {t('site.noneAdminCta')}
          </Link>
        ) : (
          <p className="mt-3 text-sm text-gray-600">{t('site.noneContactAdmin')}</p>
        )}
      </div>
    )
  }
  return <>{children(site)}</>
}
