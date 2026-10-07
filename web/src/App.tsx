import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Layout } from './components/Layout'
import { ProtectedRoute } from './components/ProtectedRoute'
import { ErrorBoundary } from './components/ErrorBoundary'
import { ToastProvider } from './components/toast/ToastProvider'
import { SiteProvider } from './context/SiteProvider'
import { apiErrorStatus } from './api/client'
import Dashboard from './pages/Dashboard'
import Attendance from './pages/Attendance'
import Schedules from './pages/Schedules'
import Guards from './pages/Guards'
import Patrol from './pages/Patrol'
import Incidents from './pages/Incidents'
import Reports from './pages/Reports'
import Settings from './pages/Settings'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      // Don't retry client errors (403/404/400); retry transient ones once.
      retry: (count, err) => {
        const status = apiErrorStatus(err)
        return count < 1 && (status === undefined || status >= 500)
      },
      refetchOnWindowFocus: false,
    },
  },
})

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <SiteProvider>
          <BrowserRouter>
            <ErrorBoundary>
              <Routes>
                <Route element={<Layout />}>
                  <Route index element={<ProtectedRoute cap="viewDashboard"><Dashboard /></ProtectedRoute>} />
                  <Route path="/attendance" element={<ProtectedRoute cap="viewAttendance"><Attendance /></ProtectedRoute>} />
                  <Route path="/schedules" element={<ProtectedRoute cap="viewSchedules"><Schedules /></ProtectedRoute>} />
                  <Route path="/guards" element={<ProtectedRoute cap="viewGuards"><Guards /></ProtectedRoute>} />
                  <Route path="/patrol" element={<ProtectedRoute cap="viewPatrol"><Patrol /></ProtectedRoute>} />
                  <Route path="/incidents" element={<ProtectedRoute cap="viewIncidents"><Incidents /></ProtectedRoute>} />
                  <Route path="/reports" element={<ProtectedRoute cap="viewReports"><Reports /></ProtectedRoute>} />
                  <Route path="/settings" element={<ProtectedRoute cap="viewSettings"><Settings /></ProtectedRoute>} />
                  <Route path="*" element={<Navigate to="/" replace />} />
                </Route>
              </Routes>
            </ErrorBoundary>
          </BrowserRouter>
        </SiteProvider>
      </ToastProvider>
    </QueryClientProvider>
  )
}
