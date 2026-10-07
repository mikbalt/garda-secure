import { useEffect, useRef, useState } from 'react'
import { NavLink, Outlet, useLocation } from 'react-router-dom'
import {
  LayoutDashboard,
  ClipboardList,
  CalendarDays,
  Users,
  MapPin,
  AlertTriangle,
  BarChart2,
  Settings,
  LogOut,
  Shield,
  Menu,
  X,
  ChevronDown,
  Building2,
  Repeat,
} from 'lucide-react'
import { useAuth } from '../hooks/useAuth'
import { useI18n, type Key } from '../i18n/context'
import { useSite } from '../context/site'
import type { Capability } from '../lib/roles'
import type { Lang } from '../lib/time'
import { SosBanner } from './SosBanner'

const navItems: { to: string; label: Key; icon: typeof Menu; cap: Capability; end?: boolean }[] = [
  { to: '/', label: 'nav.dashboard', icon: LayoutDashboard, cap: 'viewDashboard', end: true },
  { to: '/attendance', label: 'nav.attendance', icon: ClipboardList, cap: 'viewAttendance' },
  { to: '/schedules', label: 'nav.schedules', icon: CalendarDays, cap: 'viewSchedules' },
  { to: '/guards', label: 'nav.guards', icon: Users, cap: 'viewGuards' },
  { to: '/patrol', label: 'nav.patrol', icon: MapPin, cap: 'viewPatrol' },
  { to: '/incidents', label: 'nav.incidents', icon: AlertTriangle, cap: 'viewIncidents' },
  { to: '/reports', label: 'nav.reports', icon: BarChart2, cap: 'viewReports' },
  { to: '/settings', label: 'nav.settings', icon: Settings, cap: 'viewSettings' },
]

function SiteSelector() {
  const { sites, siteId, setSiteId, isLoading } = useSite()
  const { t } = useI18n()
  return (
    <div className="flex items-center gap-2">
      <Building2 className="h-4 w-4 text-gray-600" aria-hidden="true" />
      <label htmlFor="global-site" className="sr-only">
        {t('site.selector')}
      </label>
      <select
        id="global-site"
        value={siteId}
        onChange={(e) => setSiteId(e.target.value)}
        disabled={isLoading || sites.length === 0}
        className="max-w-[14rem] rounded-md border border-gray-300 bg-white py-1.5 pl-2 pr-8 text-sm text-gray-900 focus:border-primary-600 focus:outline-none focus:ring-2 focus:ring-primary-600/30"
      >
        {isLoading && <option value="">{t('site.loading')}</option>}
        {!isLoading && sites.length === 0 && <option value="">{t('site.none')}</option>}
        {sites.map((s) => (
          <option key={s.id} value={s.id}>
            {s.name}
          </option>
        ))}
      </select>
    </div>
  )
}

function LanguageSwitcher() {
  const { lang, setLang, t } = useI18n()
  return (
    <div>
      <label htmlFor="lang-switch" className="sr-only">
        {t('lang.label')}
      </label>
      <select
        id="lang-switch"
        value={lang}
        onChange={(e) => setLang(e.target.value as Lang)}
        className="rounded-md border border-gray-300 bg-white py-1.5 pl-2 pr-7 text-sm text-gray-900 focus:border-primary-600 focus:outline-none focus:ring-2 focus:ring-primary-600/30"
      >
        <option value="id">ID — {t('lang.id')}</option>
        <option value="en">EN — {t('lang.en')}</option>
      </select>
    </div>
  )
}

function UserMenu() {
  const { user, logout, switchOrganisation, multiRealm } = useAuth()
  const { t } = useI18n()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const onDoc = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false)
    document.addEventListener('mousedown', onDoc)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDoc)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  const item =
    'flex w-full items-center gap-2 px-3 py-2 text-left text-sm text-gray-800 hover:bg-gray-100 focus:bg-gray-100 focus:outline-none'

  return (
    <div className="relative" ref={ref}>
      <button
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={t('user.menu')}
        onClick={() => setOpen((o) => !o)}
        className="flex items-center gap-1.5 rounded-md px-2 py-1.5 text-sm text-gray-800 hover:bg-gray-100 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-600"
      >
        <span className="max-w-[10rem] truncate font-medium">{user.username}</span>
        <ChevronDown className="h-4 w-4" aria-hidden="true" />
      </button>
      {open && (
        <div role="menu" className="absolute right-0 z-40 mt-1 w-64 overflow-hidden rounded-md border border-gray-200 bg-white py-1 shadow-lg">
          <div className="border-b border-gray-100 px-3 py-2 text-xs text-gray-600">
            <div>
              {t('org.current')}: <span className="font-medium text-gray-900">{user.realmLabel}</span>
            </div>
            <div className="mt-0.5 truncate">{user.roles.filter((r) => !r.startsWith('default-') && r !== 'offline_access' && r !== 'uma_authorization').join(', ')}</div>
          </div>
          {multiRealm && (
            <button type="button" role="menuitem" className={item} onClick={switchOrganisation}>
              <Repeat className="h-4 w-4" aria-hidden="true" />
              {t('org.switch')}
            </button>
          )}
          <button type="button" role="menuitem" className={`${item} text-red-800`} onClick={logout}>
            <LogOut className="h-4 w-4" aria-hidden="true" />
            {t('user.logout')}
          </button>
        </div>
      )}
    </div>
  )
}

export function Layout() {
  const { can } = useAuth()
  const { t } = useI18n()
  const [menuOpen, setMenuOpen] = useState(false)
  const location = useLocation()

  useEffect(() => setMenuOpen(false), [location.pathname])

  return (
    <div className="flex h-screen bg-gray-50 font-sans">
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:absolute focus:left-2 focus:top-2 focus:z-50 focus:rounded focus:bg-white focus:px-3 focus:py-2 focus:text-primary-800"
      >
        {t('app.skipToContent')}
      </a>

      {/* Sidebar */}
      <aside
        className={`no-print fixed inset-y-0 left-0 z-40 flex w-60 shrink-0 flex-col bg-primary-800 transition-transform lg:static lg:translate-x-0 ${
          menuOpen ? 'translate-x-0' : '-translate-x-full'
        }`}
      >
        <div className="flex items-center gap-2 border-b border-primary-700 px-5 py-5">
          <Shield className="h-6 w-6 text-white" aria-hidden="true" />
          <span className="text-lg font-bold tracking-tight text-white">{t('app.name')}</span>
          <button
            type="button"
            className="ml-auto rounded p-1 text-primary-100 hover:bg-primary-700 lg:hidden"
            onClick={() => setMenuOpen(false)}
            aria-label={t('nav.closeMenu')}
          >
            <X className="h-5 w-5" aria-hidden="true" />
          </button>
        </div>
        <nav aria-label={t('nav.main')} className="flex-1 overflow-y-auto py-4">
          {navItems
            .filter((n) => can(n.cap))
            .map(({ to, label, icon: Icon, end }) => (
              <NavLink
                key={to}
                to={to}
                end={end}
                className={({ isActive }) =>
                  `flex items-center gap-3 px-5 py-3 text-sm font-medium transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-white ${
                    isActive ? 'bg-primary-900 text-white' : 'text-primary-100 hover:bg-primary-700 hover:text-white'
                  }`
                }
              >
                <Icon className="h-4 w-4 shrink-0" aria-hidden="true" />
                {t(label)}
              </NavLink>
            ))}
        </nav>
        <div className="border-t border-primary-700 px-5 py-4 text-xs text-primary-200">v1.0.0</div>
      </aside>
      {menuOpen && <div className="fixed inset-0 z-30 bg-gray-900/40 lg:hidden" onClick={() => setMenuOpen(false)} aria-hidden="true" />}

      {/* Main area */}
      <div className="flex flex-1 flex-col overflow-hidden">
        <header className="no-print flex shrink-0 flex-wrap items-center gap-3 border-b border-gray-200 bg-white px-4 py-2.5 sm:px-6">
          <button
            type="button"
            className="rounded p-1.5 text-gray-700 hover:bg-gray-100 lg:hidden"
            onClick={() => setMenuOpen(true)}
            aria-label={t('nav.openMenu')}
            aria-expanded={menuOpen}
          >
            <Menu className="h-5 w-5" aria-hidden="true" />
          </button>
          <SiteSelector />
          <div className="ml-auto flex items-center gap-3">
            <LanguageSwitcher />
            <UserMenu />
          </div>
        </header>
        <SosBanner />
        <main id="main" tabIndex={-1} className="flex-1 overflow-auto p-4 focus:outline-none sm:p-6">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
