import { lazy, Suspense, type ReactNode } from 'react'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Layout } from './components/Layout'
import { ProtectedRoute } from './components/ProtectedRoute'
import { ErrorBoundary } from './components/ErrorBoundary'
import { ToastProvider } from './components/toast/ToastProvider'
import { SiteProvider } from './context/SiteProvider'
import { retryDelay, shouldRetry } from './api/client'
import { TableSkeleton } from './components/LoadingSkeleton'
import { useAuth } from './hooks/useAuth'
import type { Capability } from './lib/roles'

// Pages are code-split so the first screen loads only what it needs.
const Dashboard = lazy(() => import('./pages/Dashboard'))
const Attendance = lazy(() => import('./pages/Attendance'))
const Schedules = lazy(() => import('./pages/Schedules'))
const Guards = lazy(() => import('./pages/Guards'))
const Patrol = lazy(() => import('./pages/Patrol'))
const Incidents = lazy(() => import('./pages/Incidents'))
const Reports = lazy(() => import('./pages/Reports'))
const Settings = lazy(() => import('./pages/Settings'))
const Setup = lazy(() => import('./pages/Setup'))
const Audit = lazy(() => import('./pages/Audit'))

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      // Client errors are final; transient ones are retried once, a 429
      // after its Retry-After.
      retry: shouldRetry,
      retryDelay,
      refetchOnWindowFocus: false,
    },
  },
})

/** Landing page: "Today" for realm roles, Settings for a platform operator. */
function Home() {
  const { can } = useAuth()
  if (!can('viewDashboard') && can('viewSettings')) return <Navigate to="/settings" replace />
  return (
    <ProtectedRoute cap="viewDashboard">
      <Dashboard />
    </ProtectedRoute>
  )
}

const page = (cap: Capability, el: ReactNode) => <ProtectedRoute cap={cap}>{el}</ProtectedRoute>

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <SiteProvider>
          <BrowserRouter>
            {/* Shell-level boundary; each route has its own inside the Layout. */}
            <ErrorBoundary>
              <Suspense fallback={<TableSkeleton />}>
                <Routes>
                  <Route element={<Layout />}>
                    <Route index element={<Home />} />
                    <Route path="/attendance" element={page('viewAttendance', <Attendance />)} />
                    <Route path="/schedules" element={page('viewSchedules', <Schedules />)} />
                    <Route path="/guards" element={page('viewGuards', <Guards />)} />
                    <Route path="/patrol" element={page('viewPatrol', <Patrol />)} />
                    <Route path="/incidents" element={page('viewIncidents', <Incidents />)} />
                    <Route path="/reports" element={page('viewReports', <Reports />)} />
                    <Route path="/setup" element={page('viewSetup', <Setup />)} />
                    <Route path="/settings" element={page('viewSettings', <Settings />)} />
                    <Route path="/audit" element={page('viewAudit', <Audit />)} />
                    <Route path="*" element={<Navigate to="/" replace />} />
                  </Route>
                </Routes>
              </Suspense>
            </ErrorBoundary>
          </BrowserRouter>
        </SiteProvider>
      </ToastProvider>
    </QueryClientProvider>
  )
}
