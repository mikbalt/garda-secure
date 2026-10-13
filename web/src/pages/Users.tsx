import { useEffect, useState } from 'react'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, Search, ShieldCheck, Smartphone, UserCog, UserPlus, X } from 'lucide-react'
import apiClient from '../api/client'
import { TableSkeleton } from '../components/LoadingSkeleton'
import { Badge } from '../components/ui/Badge'
import { Button } from '../components/ui/Button'
import { Input, Select } from '../components/ui/Form'
import { EmptyState, ErrorState, Notice, PageHeader } from '../components/ui/States'
import { useErrorToast, useToast } from '../components/toast/context'
import { useSite } from '../context/site'
import { useAuth } from '../hooks/useAuth'
import { useMe } from '../hooks/useMe'
import { useNewParam } from '../hooks/useNewParam'
import { useI18n, type Key } from '../i18n/context'
import { FilterChips, PersonCell, type ChipItem } from './guards/PeopleKit'
import { ConfirmDialog } from './settings/kit'
import { ResetPasswordModal } from './users/ResetPasswordModal'
import { UserCreateModal } from './users/UserCreateModal'
import { UserDrawer, type UserAction } from './users/UserDrawer'
import { RoleBadges, SitesSummary } from './users/UserFields'
import { REALM_ROLES, displayName, otpMissing, pendingActions, requiredActionLabel } from './users/userRules'
import type { ManagedUser } from '../types'

const PAGE = 25
type Status = 'all' | 'enabled' | 'disabled'

/** Debounced value (server-side search). */
function useDebounced<T>(value: T, ms = 300): T {
  const [v, setV] = useState(value)
  useEffect(() => {
    const id = setTimeout(() => setV(value), ms)
    return () => clearTimeout(id)
  }, [value, ms])
  return v
}

/** Sign-in state: OTP + pending required actions. */
function SignInCell({ user }: { user: ManagedUser }) {
  const { t } = useI18n()
  const actions = pendingActions(user)
  return (
    <span className="flex flex-wrap gap-1">
      {user.otp_configured ? (
        <Badge tone="green">
          <Smartphone className="h-3 w-3" aria-hidden="true" />
          {t('acc.usr.otpOn')}
        </Badge>
      ) : otpMissing(user) && !actions.includes('CONFIGURE_TOTP') ? (
        <Badge tone="yellow">{t('acc.usr.otpPending')}</Badge>
      ) : null}
      {actions.map((a) => (
        <Badge key={a} tone="yellow">
          {requiredActionLabel(a, t)}
        </Badge>
      ))}
      {user.otp_configured === false && actions.length === 0 && !otpMissing(user) && <span className="text-xs text-gray-500">{t('acc.usr.passwordOnly')}</span>}
    </span>
  )
}

export default function Users() {
  const { t } = useI18n()
  const { user: auth } = useAuth()
  const { sites } = useSite()
  const me = useMe()
  const qc = useQueryClient()
  const toast = useToast()
  const onError = useErrorToast()
  const [search, setSearch] = useState('')
  const [role, setRole] = useState('')
  const [status, setStatus] = useState<Status>('all')
  const [first, setFirst] = useState(0)
  const [creating, setCreating] = useState(false)
  const [open, setOpen] = useState<ManagedUser | null>(null)
  const [action, setAction] = useState<{ kind: UserAction; user: ManagedUser } | null>(null)
  const q = useDebounced(search.trim())

  const callerRoles = me.data?.roles?.length ? me.data.roles : auth.roles
  const restricted = !!me.data?.sites_restricted
  const selfId = me.data?.subject

  useNewParam(() => setCreating(true))
  useEffect(() => setFirst(0), [q, role, status])

  const { data, isLoading, isFetching, error, refetch } = useQuery({
    queryKey: ['users', { q, role, status, first }],
    queryFn: async () => {
      const res = await apiClient.get<ManagedUser[]>('/users', {
        params: { search: q || undefined, role: role || undefined, enabled: status === 'all' ? undefined : String(status === 'enabled'), first, max: PAGE },
      })
      const total = Number(res.headers['x-total-count'])
      return { rows: res.data ?? [], total: Number.isFinite(total) ? total : (res.data ?? []).length }
    },
    placeholderData: keepPreviousData,
  })

  const resetOtp = useMutation({
    mutationFn: (u: ManagedUser) => apiClient.post(`/users/${u.id}/reset-otp`),
    onSuccess: (_r, u) => {
      toast.success(t('acc.usr.otpResetDone', { name: displayName(u) }))
      qc.invalidateQueries({ queryKey: ['users'] })
      setAction(null)
    },
    onError,
  })
  const logout = useMutation({
    mutationFn: (u: ManagedUser) => apiClient.post(`/users/${u.id}/logout`),
    onSuccess: (_r, u) => {
      toast.success(t('acc.usr.logoutDone', { name: displayName(u) }))
      setAction(null)
    },
    onError,
  })

  const rows = data?.rows ?? []
  const total = data?.total ?? 0
  const filtered = !!(q || role || status !== 'all')
  const clear = () => {
    setSearch('')
    setRole('')
    setStatus('all')
  }
  const chips: ChipItem<Status>[] = [
    { key: 'all', label: t('common.all') },
    { key: 'enabled', label: t('acc.usr.enabled'), dot: 'bg-emerald-500' },
    { key: 'disabled', label: t('acc.usr.disabled'), dot: 'bg-red-500' },
  ]
  const manage = (u: ManagedUser) => (
    <Button size="sm" variant="secondary" onClick={() => setOpen(u)} aria-label={`${t('acc.usr.manage')} — ${displayName(u)}`}>
      <UserCog className="h-3.5 w-3.5" aria-hidden="true" />
      <span className="hidden xl:inline">{t('acc.usr.manage')}</span>
    </Button>
  )
  const person = (u: ManagedUser) => (
    <PersonCell
      name={displayName(u)}
      sub={[u.username, u.email].filter(Boolean).join(' · ')}
      extra={
        <>
          {u.id === selfId && <Badge tone="blue">{t('acc.usr.you')}</Badge>}
          {!u.enabled && <Badge tone="red" dot>{t('acc.usr.disabled')}</Badge>}
        </>
      }
    />
  )

  let body
  if (isLoading) body = <TableSkeleton />
  else if (error) body = <ErrorState error={error} onRetry={() => refetch()} />
  else if (rows.length === 0 && !filtered)
    body = (
      <EmptyState icon={UserCog} title={t('acc.usr.emptyTitle')} message={t('acc.usr.emptyBody')}>
        <Button onClick={() => setCreating(true)}>
          <UserPlus className="h-4 w-4" aria-hidden="true" />
          {t('acc.usr.add')}
        </Button>
      </EmptyState>
    )
  else if (rows.length === 0)
    body = (
      <EmptyState icon={Search} title={t('acc.usr.noMatchTitle')} message={t('acc.usr.noMatchBody')}>
        <Button variant="secondary" onClick={clear}>
          {t('ppl.grd.clearFilters')}
        </Button>
      </EmptyState>
    )
  else
    body = (
      <>
        <div
          tabIndex={0}
          role="region"
          aria-label={t('acc.usr.title')}
          className="scroll-thin relative hidden overflow-x-auto rounded-2xl border border-gray-200/80 bg-white shadow-card focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 md:block"
        >
          <table className="min-w-full text-sm" aria-busy={isFetching || undefined}>
            <caption className="sr-only">{t('acc.usr.title')}</caption>
            <thead className="border-b border-gray-200 bg-gray-50/80">
              <tr className="text-left text-[11px] font-semibold uppercase tracking-wider text-gray-500">
                <th scope="col" className="px-5 py-2.5">{t('acc.usr.person')}</th>
                <th scope="col" className="px-4 py-2.5">{t('acc.usr.roles')}</th>
                <th scope="col" className="px-4 py-2.5">{t('acc.usr.sites')}</th>
                <th scope="col" className="px-4 py-2.5">{t('acc.usr.signIn')}</th>
                <th scope="col" className="px-4 py-2.5">
                  <span className="sr-only">{t('common.actions')}</span>
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {rows.map((u) => (
                <tr key={u.id} className={`transition-colors hover:bg-gray-50/80 ${u.enabled ? '' : 'bg-gray-50/60'}`}>
                  <td className="max-w-[18rem] px-5 py-3">{person(u)}</td>
                  <td className="px-4 py-3">
                    <RoleBadges roles={u.roles} />
                  </td>
                  <td className="max-w-[14rem] truncate px-4 py-3">
                    <SitesSummary ids={u.sites} sites={sites} />
                  </td>
                  <td className="min-w-[10rem] max-w-[13rem] px-4 py-3">
                    <SignInCell user={u} />
                  </td>
                  <td className="px-4 py-3 text-right">{manage(u)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <ul className="space-y-3 md:hidden" aria-label={t('acc.usr.title')}>
          {rows.map((u) => (
            <li key={u.id} className="rounded-2xl border border-gray-200/80 bg-white p-4 shadow-card">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">{person(u)}</div>
                {manage(u)}
              </div>
              <div className="mt-3 space-y-2 text-sm">
                <RoleBadges roles={u.roles} />
                <p className="truncate">
                  <SitesSummary ids={u.sites} sites={sites} />
                </p>
                <SignInCell user={u} />
              </div>
            </li>
          ))}
        </ul>
        <nav aria-label={t('acc.usr.paging')} className="mt-3 flex flex-wrap items-center justify-between gap-3">
          <p className="text-xs text-gray-500" aria-live="polite">
            {t('acc.usr.showing', { from: first + 1, to: first + rows.length, total })}
          </p>
          {total > PAGE && (
            <div className="flex gap-2">
              <Button size="sm" variant="secondary" disabled={first === 0} onClick={() => setFirst(Math.max(0, first - PAGE))}>
                <ChevronLeft className="h-3.5 w-3.5" aria-hidden="true" />
                {t('acc.usr.prev')}
              </Button>
              <Button size="sm" variant="secondary" disabled={first + PAGE >= total} onClick={() => setFirst(first + PAGE)}>
                {t('acc.usr.next')}
                <ChevronRight className="h-3.5 w-3.5" aria-hidden="true" />
              </Button>
            </div>
          )}
        </nav>
      </>
    )

  return (
    <>
      <PageHeader
        icon={UserCog}
        title={t('acc.usr.title')}
        subtitle={t('acc.usr.subtitle')}
        actions={
          <Button onClick={() => setCreating(true)}>
            <UserPlus className="h-4 w-4" aria-hidden="true" />
            {t('acc.usr.add')}
          </Button>
        }
      />
      {restricted && <Notice>{t('acc.usr.restrictedNotice', { n: me.data?.sites?.length ?? 0 })}</Notice>}

      <div className="no-print mb-4 flex flex-col gap-3 rounded-2xl border border-gray-200/80 bg-white px-4 py-3 shadow-card lg:flex-row lg:items-center">
        <div className="relative lg:w-72">
          <label htmlFor="user-search" className="sr-only">
            {t('acc.usr.searchLabel')}
          </label>
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-500" aria-hidden="true" />
          <Input
            id="user-search"
            type="search"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t('acc.usr.searchPlaceholder')}
            className="pl-9 pr-9"
            autoComplete="off"
          />
          {search && (
            <button
              type="button"
              onClick={() => setSearch('')}
              aria-label={t('ppl.grd.clearSearch')}
              className="absolute right-2 top-1/2 -translate-y-1/2 rounded p-1 text-gray-500 hover:text-gray-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
            >
              <X className="h-4 w-4" aria-hidden="true" />
            </button>
          )}
        </div>
        <div className="lg:w-56">
          <label htmlFor="user-role" className="sr-only">
            {t('acc.usr.roleFilter')}
          </label>
          <Select id="user-role" value={role} onChange={(e) => setRole(e.target.value)}>
            <option value="">{t('acc.usr.allRoles')}</option>
            {[...REALM_ROLES, ...(callerRoles.includes('platform-admin') ? ['platform-admin'] : [])].map((r) => (
              <option key={r} value={r}>
                {t(`role.${r}` as Key)}
              </option>
            ))}
          </Select>
        </div>
        <FilterChips label={t('acc.usr.statusFilter')} items={chips} active={status} onChange={setStatus} />
        <p className="flex items-center gap-1.5 text-xs text-gray-500 lg:ml-auto">
          <ShieldCheck className="h-3.5 w-3.5" aria-hidden="true" />
          {t('acc.usr.auditNote')}
        </p>
      </div>

      {body}

      {creating && <UserCreateModal callerRoles={callerRoles} callerRestricted={restricted} onClose={() => setCreating(false)} />}
      {open && (
        <UserDrawer
          user={open}
          isSelf={open.id === selfId}
          callerRoles={callerRoles}
          callerRestricted={restricted}
          onClose={() => setOpen(null)}
          onAction={(kind, u) => {
            setOpen(null)
            setAction({ kind, user: u })
          }}
        />
      )}
      {action?.kind === 'reset-password' && <ResetPasswordModal user={action.user} onClose={() => setAction(null)} />}
      <ConfirmDialog
        open={action?.kind === 'reset-otp'}
        title={t('acc.usr.resetOtpTitle', { name: action ? displayName(action.user) : '' })}
        message={t('acc.usr.resetOtpBody')}
        confirmLabel={t('acc.usr.resetOtp')}
        loading={resetOtp.isPending}
        onConfirm={() => action && resetOtp.mutate(action.user)}
        onClose={() => setAction(null)}
      />
      <ConfirmDialog
        open={action?.kind === 'logout'}
        title={t('acc.usr.logoutTitle', { name: action ? displayName(action.user) : '' })}
        message={t('acc.usr.logoutBody')}
        confirmLabel={t('acc.usr.logout')}
        loading={logout.isPending}
        onConfirm={() => action && logout.mutate(action.user)}
        onClose={() => setAction(null)}
      />
    </>
  )
}
