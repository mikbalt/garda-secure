import { Suspense, useCallback, useEffect, useRef, useState } from 'react'
import { Link, NavLink, Outlet, useLocation } from 'react-router-dom'
import {
  BarChart3,
  Bell,
  Building2,
  CalendarDays,
  Check,
  ChevronsUpDown,
  ClipboardCheck,
  FileClock,
  LayoutGrid,
  ListChecks,
  LogOut,
  Menu,
  Repeat,
  Route,
  Settings2,
  ShieldAlert,
  UserCog,
  ShieldCheck,
  Users,
  X,
  type LucideIcon,
} from 'lucide-react'
import { useAuth } from '../hooks/useAuth'
import { useSetupProgress } from '../hooks/useSetupProgress'
import { useMe } from '../hooks/useMe'
import { useServiceWorkerNavigation } from '../hooks/usePush'
import { useI18n, type Key } from '../i18n/context'
import { useSite } from '../context/site'
import { config } from '../config'
import { primaryRole, type Capability } from '../lib/roles'
import type { Lang } from '../lib/time'
import { SosBanner } from './SosBanner'
import { DemoBar } from './DemoBar'
import { ErrorBoundary } from './ErrorBoundary'
import { IdleLogout } from './IdleLogout'
import { TableSkeleton } from './LoadingSkeleton'
import { Popover } from './ui/Popover'

interface NavItem {
  to: string
  label: Key
  icon: LucideIcon
  cap: Capability
  end?: boolean
}

const groups: { label?: Key; items: NavItem[] }[] = [
  {
    items: [{ to: '/', label: 'nav.today', icon: LayoutGrid, cap: 'viewDashboard', end: true }],
  },
  {
    label: 'nav.group.ops',
    items: [
      { to: '/patrol', label: 'nav.patrol', icon: Route, cap: 'viewPatrol' },
      { to: '/incidents', label: 'nav.incidents', icon: ShieldAlert, cap: 'viewIncidents' },
      { to: '/attendance', label: 'nav.attendance', icon: ClipboardCheck, cap: 'viewAttendance' },
    ],
  },
  {
    label: 'nav.group.people',
    items: [
      { to: '/schedules', label: 'nav.schedules', icon: CalendarDays, cap: 'viewSchedules' },
      { to: '/guards', label: 'nav.guards', icon: Users, cap: 'viewGuards' },
    ],
  },
  {
    label: 'nav.group.insight',
    items: [{ to: '/reports', label: 'nav.reports', icon: BarChart3, cap: 'viewReports' }],
  },
  {
    label: 'nav.group.admin',
    items: [
      { to: '/setup', label: 'nav.setup', icon: ListChecks, cap: 'viewSetup' },
      { to: '/settings', label: 'nav.settings', icon: Settings2, cap: 'viewSettings' },
      { to: '/users', label: 'nav.users', icon: UserCog, cap: 'manageUsers' },
      { to: '/audit', label: 'nav.audit', icon: FileClock, cap: 'viewAudit' },
    ],
  },
]

function initials(name: string) {
  return (
    name
      .replace(/^demo\./, '')
      .split(/[\s._-]+/)
      .filter(Boolean)
      .slice(0, 2)
      .map((p) => p[0]?.toUpperCase())
      .join('') || '?'
  )
}

function Brand() {
  const { t } = useI18n()
  const { user } = useAuth()
  return (
    <div className="flex min-w-0 flex-1 items-center gap-3 px-5 pb-5 pt-6">
      <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-gradient-to-br from-primary-400 to-primary-700 shadow-lg shadow-primary-900/40 ring-1 ring-white/20">
        <ShieldCheck className="h-5 w-5 text-white" aria-hidden="true" />
      </div>
      <div className="min-w-0">
        <p className="text-[15px] font-semibold leading-5 tracking-tight text-white">{t('app.name')}</p>
        <p className="truncate text-xs text-gray-300">{user.realmLabel}</p>
      </div>
    </div>
  )
}

function SetupMiniCard() {
  const { t } = useI18n()
  const s = useSetupProgress()
  if (!s.enabled || s.isLoading || s.complete || !s.next) return null
  const pct = Math.round((s.done / s.total) * 100)
  return (
    <Link
      to="/setup"
      className="mx-3 mb-3 mt-auto block shrink-0 rounded-xl bg-white/[0.06] p-3.5 ring-1 ring-white/10 transition-colors hover:bg-white/[0.09] focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-400"
    >
      <div className="flex items-center justify-between text-xs font-medium text-gray-200">
        <span>{t('setup.continue')}</span>
        <span className="tabular-nums text-gray-300">
          {s.done}/{s.total}
        </span>
      </div>
      <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-white/10">
        <div className="h-full rounded-full bg-gradient-to-r from-primary-400 to-emerald-400" style={{ width: `${pct}%` }} />
      </div>
      <p className="mt-2 line-clamp-2 [@media(max-height:52rem)]:hidden text-xs leading-snug text-gray-300">{t('setup.cardBody', { step: t(s.next.title) })}</p>
    </Link>
  )
}

/** "Limited to N locations" for a site-scoped user (D-042), with the names as a tooltip. */
function ScopeHint() {
  const { t } = useI18n()
  const { sites } = useSite()
  const { data } = useMe()
  if (!data?.sites_restricted) return null
  const n = data.sites?.length ?? 0
  const names = (data.sites ?? []).map((id) => sites.find((s) => s.id === id)?.name).filter(Boolean)
  return (
    <p className="mt-1 inline-flex items-center gap-1 rounded-md bg-primary-50 px-1.5 py-0.5 text-xs text-primary-800" title={names.join(', ') || undefined}>
      <Building2 className="h-3 w-3" aria-hidden="true" />
      {t('acc.scope.limited', { n })}
    </p>
  )
}

function UserPanel() {
  const { user, logout, switchOrganisation, multiRealm, can } = useAuth()
  const { t, lang, setLang } = useI18n()
  const role = primaryRole(user.roles)
  const roleLabel = role ? t(`role.${role}` as Key) : ''
  const item =
    'flex w-full items-center gap-2.5 px-3.5 py-2 text-left text-sm text-gray-700 transition-colors hover:bg-gray-50 focus:bg-gray-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500'
  return (
    <Popover
      side="top"
      label={t('user.menu')}
      className="px-3 pb-4"
      panelClassName="left-3 right-3 min-w-0"
      trigger={({ toggle, open: _open, ...aria }) => (
        <button
          type="button"
          onClick={toggle}
          aria-label={t('user.menu')}
          {...aria}
          className="flex w-full items-center gap-3 rounded-xl px-2.5 py-2 text-left transition-colors hover:bg-white/[0.06] focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-400"
        >
          <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-gray-600 to-gray-700 text-xs font-semibold text-white ring-1 ring-white/10">
            {initials(user.username)}
          </span>
          <span className="min-w-0 flex-1">
            <span className="block truncate text-sm font-medium text-white">{user.username}</span>
            <span className="block truncate text-xs text-gray-300">{roleLabel}</span>
          </span>
          <ChevronsUpDown className="h-4 w-4 shrink-0 text-gray-400" aria-hidden="true" />
        </button>
      )}
    >
      {(close) => (
        <>
          <div className="border-b border-gray-100 px-3.5 pb-2.5 pt-1">
            <p className="text-xs text-gray-600">{t('shell.signedInAs')}</p>
            <p className="truncate text-sm font-medium text-gray-900">{user.username}</p>
            <p className="truncate text-xs text-gray-600">
              {roleLabel} · {user.realmLabel}
            </p>
            <ScopeHint />
          </div>
          <div className="px-3.5 py-2.5">
            <p className="mb-1.5 text-xs font-medium text-gray-600">{t('shell.language')}</p>
            <div role="group" aria-label={t('shell.language')} className="grid grid-cols-2 gap-1 rounded-lg bg-gray-100 p-1">
              {(['id', 'en'] as Lang[]).map((l) => (
                <button
                  key={l}
                  type="button"
                  aria-pressed={lang === l}
                  onClick={() => setLang(l)}
                  className={`rounded-md py-1 text-xs font-medium transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 ${
                    lang === l ? 'bg-white text-gray-900 shadow-card' : 'text-gray-700 hover:text-gray-900'
                  }`}
                >
                  {l === 'id' ? t('lang.id') : t('lang.en')}
                </button>
              ))}
            </div>
          </div>
          <div className="border-t border-gray-100 pt-1">
            {can('viewNotifications') && (
              <Link to="/notifications" className={item} onClick={close}>
                <Bell className="h-4 w-4 text-gray-500" aria-hidden="true" />
                {t('push.title')}
              </Link>
            )}
            {multiRealm && (
              <button type="button" className={item} onClick={() => (close(), switchOrganisation())}>
                <Repeat className="h-4 w-4 text-gray-500" aria-hidden="true" />
                {t('org.switch')}
              </button>
            )}
            <button type="button" className={`${item} text-red-700`} onClick={() => (close(), logout())}>
              <LogOut className="h-4 w-4" aria-hidden="true" />
              {t('user.logout')}
            </button>
          </div>
        </>
      )}
    </Popover>
  )
}

function Sidebar({ onClose }: { onClose: () => void }) {
  const { can } = useAuth()
  const { t } = useI18n()
  return (
    <div className="flex h-full flex-col bg-ink-900 bg-[radial-gradient(120%_60%_at_0%_0%,rgba(61,104,243,0.18),transparent_60%)]">
      <div className="flex items-start">
        <Brand />
        <button
          type="button"
          className="mr-3 mt-6 shrink-0 rounded-lg p-1.5 text-gray-300 hover:bg-white/10 hover:text-white focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-400 lg:hidden"
          onClick={onClose}
          aria-label={t('nav.closeMenu')}
        >
          <X className="h-5 w-5" aria-hidden="true" />
        </button>
      </div>
      {/* Nav and the setup card share one scroll area, so the card never covers the
          last nav items on short screens (e.g. 1366x820); with room to spare the card
          sits at the bottom (`mt-auto`). Below 52rem of height the nav tightens and the
          card drops its description so both still fit at 1366x820. */}
      <div className="scroll-thin-dark flex min-h-0 flex-1 flex-col overflow-y-auto">
        <nav aria-label={t('nav.main')} className="space-y-5 px-3 pb-4 [@media(max-height:52rem)]:space-y-4">
          {groups.map((g, gi) => {
            const items = g.items.filter((n) => can(n.cap))
            if (items.length === 0) return null
            return (
              <div key={gi}>
                {g.label && <p className="mb-1.5 px-3 text-[11px] font-semibold uppercase tracking-wider text-gray-400">{t(g.label)}</p>}
                <ul className="space-y-0.5">
                  {items.map(({ to, label, icon: Icon, end }) => (
                    <li key={to}>
                      <NavLink
                        to={to}
                        end={end}
                        className={({ isActive }) =>
                          `group flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-400 ${
                            isActive ? 'bg-white/10 text-white shadow-[inset_2px_0_0_0] shadow-primary-400' : 'text-gray-300 hover:bg-white/[0.05] hover:text-white'
                          }`
                        }
                      >
                        {({ isActive }) => (
                          <>
                            <Icon className={`h-[18px] w-[18px] shrink-0 ${isActive ? 'text-primary-300' : 'text-gray-400 group-hover:text-gray-200'}`} aria-hidden="true" />
                            {t(label)}
                          </>
                        )}
                      </NavLink>
                    </li>
                  ))}
                </ul>
              </div>
            )
          })}
        </nav>
        <SetupMiniCard />
      </div>
      <div className="border-t border-white/[0.06] pt-3">
        <UserPanel />
      </div>
    </div>
  )
}

function SitePicker() {
  const { sites, site, setSiteId, isLoading } = useSite()
  const { can } = useAuth()
  const { t } = useI18n()
  if (isLoading || sites.length === 0) {
    return (
      <div className="flex items-center gap-2 text-sm text-gray-600">
        <Building2 className="h-4 w-4" aria-hidden="true" />
        {isLoading ? t('site.loading') : t('site.none')}
      </div>
    )
  }
  return (
    <Popover
      label={t('shell.sites')}
      panelClassName="w-80 max-w-[calc(100vw-2rem)]"
      trigger={({ toggle, open: _open, ...aria }) => (
        <button
          type="button"
          onClick={toggle}
          aria-label={`${t('shell.switchSite')}: ${site?.name ?? ''}`}
          {...aria}
          className="flex max-w-[18rem] items-center gap-3 rounded-xl px-2 py-1.5 text-left transition-colors hover:bg-gray-100 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
        >
          <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary-50 text-primary-600 ring-1 ring-primary-100">
            <Building2 className="h-4 w-4" aria-hidden="true" />
          </span>
          <span className="min-w-0">
            <span className="block truncate text-sm font-semibold text-gray-900">{site?.name}</span>
            {site?.address && <span className="block truncate text-xs text-gray-600">{site.address}</span>}
          </span>
          <ChevronsUpDown className="h-4 w-4 shrink-0 text-gray-500" aria-hidden="true" />
        </button>
      )}
    >
      {(close) => (
        <>
          <p className="px-3.5 pb-1.5 pt-1 text-[11px] font-semibold uppercase tracking-wider text-gray-500" aria-hidden="true">
            {t('shell.sites')}
          </p>
          {sites.map((s) => (
            <button
              key={s.id}
              type="button"
              aria-current={s.id === site?.id ? 'true' : undefined}
              onClick={() => {
                setSiteId(s.id)
                close()
              }}
              className="flex w-full items-center gap-3 px-3.5 py-2 text-left transition-colors hover:bg-gray-50 focus:bg-gray-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500"
            >
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm font-medium text-gray-900">{s.name}</span>
                {s.address && <span className="block truncate text-xs text-gray-600">{s.address}</span>}
              </span>
              {s.id === site?.id && <Check className="h-4 w-4 shrink-0 text-primary-600" aria-hidden="true" />}
            </button>
          ))}
          {can('manageSites') && (
            <Link
              to="/settings?tab=sites"
              onClick={close}
              className="mt-1 flex items-center gap-2 border-t border-gray-100 px-3.5 pb-1 pt-2.5 text-sm font-medium text-primary-700 hover:text-primary-800 focus:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500"
            >
              <Settings2 className="h-4 w-4" aria-hidden="true" />
              {t('shell.manageSites')}
            </Link>
          )}
        </>
      )}
    </Popover>
  )
}

function LangToggle() {
  const { lang, setLang, t } = useI18n()
  return (
    <div role="group" aria-label={t('lang.label')} className="hidden items-center rounded-lg bg-gray-100 p-0.5 sm:flex">
      {(['id', 'en'] as Lang[]).map((l) => (
        <button
          key={l}
          type="button"
          aria-pressed={lang === l}
          aria-label={l === 'id' ? t('lang.id') : t('lang.en')}
          onClick={() => setLang(l)}
          className={`rounded-md px-2 py-1 text-xs font-semibold uppercase transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 ${
            lang === l ? 'bg-white text-gray-900 shadow-card' : 'text-gray-600 hover:text-gray-900'
          }`}
        >
          {l}
        </button>
      ))}
    </div>
  )
}

const DESKTOP = '(min-width: 1024px)'

/** Whether the sidebar is permanently visible (Tailwind `lg`). */
function useDesktop(): boolean {
  const [desktop, setDesktop] = useState(() => typeof window !== 'undefined' && !!window.matchMedia?.(DESKTOP).matches)
  useEffect(() => {
    const mq = window.matchMedia?.(DESKTOP)
    if (!mq) return
    const on = () => setDesktop(mq.matches)
    mq.addEventListener('change', on)
    return () => mq.removeEventListener('change', on)
  }, [])
  return desktop
}

const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])'

/**
 * Off-canvas navigation below `lg` (WEB-6): `inert` and invisible while
 * closed; while open, focus moves in, Tab is trapped, Escape closes it and
 * focus returns to the menu button.
 */
function useOffCanvas(open: boolean, close: () => void, desktop: boolean) {
  const asideRef = useRef<HTMLElement>(null)
  const openerRef = useRef<HTMLButtonElement>(null)
  const modal = open && !desktop

  useEffect(() => {
    const aside = asideRef.current
    if (!aside) return
    if (!desktop && !open) aside.setAttribute('inert', '')
    else aside.removeAttribute('inert')
  }, [open, desktop])

  useEffect(() => {
    if (!modal) return
    const aside = asideRef.current
    const opener = openerRef.current
    aside?.querySelector<HTMLElement>(FOCUSABLE)?.focus()
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault()
        close()
        return
      }
      if (e.key !== 'Tab' || !aside) return
      const items = Array.from(aside.querySelectorAll<HTMLElement>(FOCUSABLE)).filter((el) => el.offsetParent !== null)
      if (items.length === 0) return
      const first = items[0]
      const last = items[items.length - 1]
      if (!aside.contains(document.activeElement)) {
        e.preventDefault()
        first.focus()
      } else if (e.shiftKey && document.activeElement === first) {
        e.preventDefault()
        last.focus()
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault()
        first.focus()
      }
    }
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('keydown', onKey)
      // Back to the menu button unless navigation moved focus elsewhere.
      if (opener && (document.activeElement === document.body || aside?.contains(document.activeElement))) opener.focus()
    }
  }, [modal, close])

  return { asideRef, openerRef, modal }
}

export function Layout() {
  const { t } = useI18n()
  const [menuOpen, setMenuOpen] = useState(false)
  const location = useLocation()
  const desktop = useDesktop()
  const closeMenu = useCallback(() => setMenuOpen(false), [])
  const { asideRef, openerRef, modal } = useOffCanvas(menuOpen, closeMenu, desktop)
  useServiceWorkerNavigation()

  useEffect(() => setMenuOpen(false), [location.pathname])

  return (
    <div className="flex h-screen bg-gray-50">
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:absolute focus:left-2 focus:top-2 focus:z-50 focus:rounded-lg focus:bg-white focus:px-3 focus:py-2 focus:text-primary-700 focus:shadow-raised"
      >
        {t('app.skipToContent')}
      </a>

      <aside
        ref={asideRef}
        id="app-sidebar"
        {...(modal ? { role: 'dialog', 'aria-modal': true, 'aria-label': t('nav.main') } : {})}
        // Opening: visible at once (so focus can move in); closing: hidden only after the slide-out.
        className={`no-print fixed inset-y-0 left-0 z-40 w-64 shrink-0 overflow-hidden duration-200 lg:visible lg:static lg:translate-x-0 ${
          menuOpen ? 'visible translate-x-0 transition-transform' : 'invisible -translate-x-full transition-[transform,visibility]'
        }`}
      >
        <Sidebar onClose={closeMenu} />
      </aside>
      {menuOpen && (
        <div className="fixed inset-0 z-30 animate-fade-in bg-gray-950/50 backdrop-blur-sm lg:hidden" onClick={closeMenu} aria-hidden="true" />
      )}

      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        {config.demo && <DemoBar />}
        {/* `relative z-20`: `backdrop-blur` makes the header a stacking context, so without
            its own z-index the later, positioned <main> (and the SOS banner) painted over the
            site picker's dropdown and swallowed its clicks. Stays below the off-canvas
            overlay (z-30), the sidebar (z-40) and modals (z-50). */}
        <header className="no-print relative z-20 flex h-16 shrink-0 items-center gap-3 border-b border-gray-200/80 bg-white/80 px-4 backdrop-blur sm:px-6">
          <button
            ref={openerRef}
            type="button"
            className="rounded-lg p-2 text-gray-600 hover:bg-gray-100 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 lg:hidden"
            onClick={() => setMenuOpen(true)}
            aria-label={t('nav.openMenu')}
            aria-expanded={menuOpen}
            aria-controls="app-sidebar"
          >
            <Menu className="h-5 w-5" aria-hidden="true" />
          </button>
          <SitePicker />
          <div className="ml-auto flex items-center gap-3">
            <LangToggle />
          </div>
        </header>
        <SosBanner />
        {!config.demo && <IdleLogout />}
        <main id="main" tabIndex={-1} className="scroll-thin relative flex-1 overflow-auto focus:outline-none">
          <div className="mx-auto w-full max-w-[1400px] px-4 py-6 sm:px-6 lg:px-8 lg:py-8">
            {/* One boundary per route (WEB-8): a broken page keeps the shell usable and recovers on navigation. */}
            <ErrorBoundary resetKey={location.pathname}>
              <Suspense fallback={<TableSkeleton />}>
                <Outlet />
              </Suspense>
            </ErrorBoundary>
          </div>
        </main>
      </div>
    </div>
  )
}
