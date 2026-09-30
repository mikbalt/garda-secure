import { NavLink, Outlet } from 'react-router-dom'
import {
  LayoutDashboard,
  ClipboardList,
  MapPin,
  AlertTriangle,
  BarChart2,
  Settings,
  LogOut,
  Shield,
} from 'lucide-react'
import { useAuth } from '../hooks/useAuth'

const navItems = [
  { to: '/',           label: 'Dashboard',  icon: LayoutDashboard, end: true },
  { to: '/attendance', label: 'Attendance', icon: ClipboardList },
  { to: '/patrol',     label: 'Patrol',     icon: MapPin },
  { to: '/incidents',  label: 'Incidents',  icon: AlertTriangle },
  { to: '/reports',    label: 'Reports',    icon: BarChart2 },
  { to: '/settings',   label: 'Settings',   icon: Settings },
]

export function Layout() {
  const { user, logout } = useAuth()

  return (
    <div className="flex h-screen bg-gray-50 font-sans">
      {/* Sidebar */}
      <aside className="w-60 bg-primary-800 flex flex-col shrink-0">
        {/* Logo */}
        <div className="flex items-center gap-2 px-5 py-5 border-b border-primary-700">
          <Shield className="text-white w-6 h-6" />
          <span className="text-white font-bold text-lg tracking-tight">SecurePatrol</span>
        </div>

        {/* Nav links */}
        <nav className="flex-1 py-4 overflow-y-auto">
          {navItems.map(({ to, label, icon: Icon, end }) => (
            <NavLink
              key={to}
              to={to}
              end={end}
              className={({ isActive }) =>
                `flex items-center gap-3 px-5 py-3 text-sm font-medium transition-colors ${
                  isActive
                    ? 'bg-primary-900 text-white'
                    : 'text-primary-200 hover:bg-primary-700 hover:text-white'
                }`
              }
            >
              <Icon className="w-4 h-4 shrink-0" />
              {label}
            </NavLink>
          ))}
        </nav>

        {/* Footer */}
        <div className="px-5 py-4 border-t border-primary-700 text-xs text-primary-300">
          v1.0.0
        </div>
      </aside>

      {/* Main area */}
      <div className="flex-1 flex flex-col overflow-hidden">
        {/* Top bar */}
        <header className="bg-white border-b border-gray-200 px-6 py-3 flex items-center justify-between shrink-0">
          <h1 className="text-gray-800 font-semibold text-sm" />
          <div className="flex items-center gap-4">
            <span className="text-sm text-gray-600">{user.username}</span>
            <button
              onClick={logout}
              className="flex items-center gap-1.5 text-sm text-gray-500 hover:text-red-600 transition-colors"
              aria-label="Log out"
            >
              <LogOut className="w-4 h-4" />
              Logout
            </button>
          </div>
        </header>

        {/* Page content */}
        <main className="flex-1 overflow-auto p-6">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
