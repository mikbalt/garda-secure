import { useId } from 'react'
import { Building2, Lock, ShieldCheck } from 'lucide-react'
import { Badge } from '../../components/ui/Badge'
import { useI18n, type Key } from '../../i18n/context'
import type { Site } from '../../types'
import { sortRoles } from './userRules'

const ROLE_TONE: Record<string, 'purple' | 'blue' | 'green' | 'gray' | 'orange'> = {
  'platform-admin': 'orange',
  admin: 'purple',
  supervisor: 'blue',
  hr: 'green',
  'client-readonly': 'gray',
}

/** Translated, colour-coded role badges (most privileged first). */
export function RoleBadges({ roles }: { roles: readonly string[] }) {
  const { t } = useI18n()
  if (roles.length === 0) return <span className="text-gray-500">—</span>
  return (
    <span className="flex flex-wrap gap-1">
      {sortRoles(roles).map((r) => (
        <Badge key={r} tone={ROLE_TONE[r] ?? 'gray'}>
          {r === 'admin' || r === 'platform-admin' ? <ShieldCheck className="h-3 w-3" aria-hidden="true" /> : null}
          {ROLE_TONE[r] ? t(`role.${r}` as Key) : r}
        </Badge>
      ))}
    </span>
  )
}

/** "All locations" or the site names (first two + "+N"). */
export function SitesSummary({ ids, sites, full = false }: { ids: readonly string[]; sites: readonly Site[]; full?: boolean }) {
  const { t } = useI18n()
  if (ids.length === 0) {
    return (
      <span className="inline-flex items-center gap-1 text-gray-700">
        <Building2 className="h-3.5 w-3.5 text-gray-500" aria-hidden="true" />
        {t('acc.usr.allSites')}
      </span>
    )
  }
  const names = ids.map((id) => sites.find((s) => s.id === id)?.name ?? t('acc.usr.unknownSite'))
  const shown = full ? names : names.slice(0, 2)
  return (
    <span className="text-gray-700" title={names.join(', ')}>
      {shown.join(', ')}
      {!full && names.length > 2 && <span className="text-gray-500"> {t('acc.usr.moreSites', { n: names.length - 2 })}</span>}
    </span>
  )
}

/** Role checkboxes with a one-line explanation each; `locked` roles cannot be removed. */
export function RolePicker({
  available,
  value,
  onChange,
  locked = [],
  disabled,
  error,
}: {
  available: readonly string[]
  value: readonly string[]
  onChange: (roles: string[]) => void
  locked?: readonly string[]
  disabled?: boolean
  error?: string
}) {
  const { t } = useI18n()
  const errId = useId()
  const toggle = (r: string, on: boolean) => onChange(on ? [...value, r] : value.filter((x) => x !== r))
  return (
    <fieldset aria-describedby={error ? errId : undefined}>
      <legend className="mb-1.5 block text-sm font-medium text-gray-700">
        {t('acc.usr.roles')}{' '}
        <span className="text-red-700" aria-hidden="true">
          *
        </span>
      </legend>
      <div className="grid gap-2 sm:grid-cols-2">
        {available.map((r) => {
          const checked = value.includes(r)
          const isLocked = locked.includes(r) && checked
          return (
            <label
              key={r}
              className={`flex cursor-pointer items-start gap-2.5 rounded-xl border p-3 text-sm transition-colors ${
                checked ? 'border-primary-300 bg-primary-50/50' : 'border-gray-200 hover:bg-gray-50'
              } ${isLocked || disabled ? 'cursor-not-allowed opacity-80' : ''}`}
            >
              <input
                type="checkbox"
                className="mt-0.5 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-2 focus:ring-primary-500"
                checked={checked}
                disabled={disabled || isLocked}
                onChange={(e) => toggle(r, e.target.checked)}
              />
              <span className="min-w-0">
                <span className="flex items-center gap-1.5 font-medium text-gray-900">
                  {t(`role.${r}` as Key)}
                  {isLocked && (
                    <>
                      <Lock className="h-3.5 w-3.5 text-gray-500" aria-hidden="true" />
                      <span className="sr-only">{t('acc.usr.roleLocked')}</span>
                    </>
                  )}
                </span>
                <span className="mt-0.5 block text-xs leading-snug text-gray-600">{t(`acc.role.${r}` as Key)}</span>
              </span>
            </label>
          )
        })}
      </div>
      {locked.length > 0 && <p className="mt-1.5 text-xs text-gray-500">{t('acc.usr.ownRolesHint')}</p>}
      {error && (
        <p id={errId} className="mt-1.5 text-xs font-medium text-red-600">
          {error}
        </p>
      )}
    </fieldset>
  )
}

/**
 * Site scope: "all locations" (empty list, also future sites) or a subset.
 * Site-scoped admins must pick from their own sites (`restricted`).
 */
export function SiteScopePicker({
  sites,
  value,
  onChange,
  restricted,
  disabled,
  error,
}: {
  sites: readonly Site[]
  value: readonly string[]
  onChange: (ids: string[]) => void
  restricted: boolean
  disabled?: boolean
  error?: string
}) {
  const { t } = useI18n()
  const name = useId()
  const errId = useId()
  const limited = value.length > 0 || restricted
  // Selecting "only these" starts with the current site list empty; keep a
  // remembered selection so toggling back and forth is harmless.
  const setMode = (only: boolean) => onChange(only ? (value.length ? [...value] : sites.slice(0, 1).map((s) => s.id)) : [])
  const toggle = (id: string, on: boolean) => onChange(on ? [...value, id] : value.filter((x) => x !== id))
  return (
    <fieldset aria-describedby={error ? errId : undefined} disabled={disabled}>
      <legend className="mb-1.5 block text-sm font-medium text-gray-700">{t('acc.usr.sites')}</legend>
      <div className="space-y-2">
        <label className={`flex items-start gap-2.5 text-sm ${restricted ? 'opacity-60' : 'cursor-pointer'}`}>
          <input
            type="radio"
            name={name}
            className="mt-0.5 h-4 w-4 border-gray-300 text-primary-600 focus:ring-2 focus:ring-primary-500"
            checked={!limited}
            disabled={restricted}
            onChange={() => setMode(false)}
          />
          <span>
            <span className="font-medium text-gray-900">{t('acc.usr.scopeAll')}</span>
            <span className="block text-xs text-gray-600">{restricted ? t('acc.usr.scopeAllRestricted') : t('acc.usr.scopeAllHint')}</span>
          </span>
        </label>
        <label className="flex cursor-pointer items-start gap-2.5 text-sm">
          <input
            type="radio"
            name={name}
            className="mt-0.5 h-4 w-4 border-gray-300 text-primary-600 focus:ring-2 focus:ring-primary-500"
            checked={limited}
            onChange={() => setMode(true)}
          />
          <span>
            <span className="font-medium text-gray-900">{t('acc.usr.scopeSome')}</span>
            <span className="block text-xs text-gray-600">{t('acc.usr.scopeSomeHint')}</span>
          </span>
        </label>
        {limited && (
          <div className="ml-6 grid gap-1.5 rounded-xl border border-gray-200 bg-gray-50/60 p-3 sm:grid-cols-2">
            {sites.map((s) => (
              <label key={s.id} className="flex cursor-pointer items-center gap-2 text-sm text-gray-800">
                <input
                  type="checkbox"
                  className="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-2 focus:ring-primary-500"
                  checked={value.includes(s.id)}
                  onChange={(e) => toggle(s.id, e.target.checked)}
                />
                <span className="truncate">{s.name}</span>
              </label>
            ))}
            {sites.length === 0 && <p className="text-sm text-gray-600">{t('site.none')}</p>}
          </div>
        )}
      </div>
      {error && (
        <p id={errId} className="mt-1.5 text-xs font-medium text-red-600">
          {error}
        </p>
      )}
    </fieldset>
  )
}
