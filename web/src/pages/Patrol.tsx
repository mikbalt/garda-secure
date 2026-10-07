import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import apiClient from '../api/client'
import { TableSkeleton } from '../components/LoadingSkeleton'
import type { Site } from '../types'
import { MapPin, CheckCircle, XCircle, Clock } from 'lucide-react'

interface Checkpoint {
  id: string
  name: string
  qr_code: string
  sla_minutes?: number
  order_index: number
}

interface Route {
  id: string
  site_id: string
  name: string
  checkpoints: Checkpoint[]
  active: boolean
  created_at: string
}

interface RouteReport {
  route_id: string
  date: string
  total_checkpoints: number
  scanned_checkpoints: number
  completion_pct: number
  missed_checkpoints: Checkpoint[]
  scans: Array<{ id: string; checkpoint_id: string; guard_id: string; scanned_at: string; method: string }>
}

export default function Patrol() {
  const today = new Date().toISOString().split('T')[0]
  const [siteFilter, setSiteFilter] = useState('')
  const [selectedRoute, setSelectedRoute] = useState<Route | null>(null)
  const [reportDate, setReportDate] = useState(today)

  const { data: sites } = useQuery<Site[]>({
    queryKey: ['sites'],
    queryFn: () => apiClient.get('/sites').then(r => r.data),
  })

  const { data: routes, isLoading } = useQuery<Route[]>({
    queryKey: ['routes', siteFilter],
    queryFn: () => {
      const p = siteFilter ? `?site_id=${siteFilter}` : ''
      return apiClient.get(`/patrol/routes${p}`).then(r => r.data)
    },
  })

  const { data: report, isLoading: reportLoading } = useQuery<RouteReport>({
    queryKey: ['route-report', selectedRoute?.id, reportDate],
    queryFn: () =>
      apiClient.get(`/patrol/routes/${selectedRoute!.id}/report?date=${reportDate}`).then(r => r.data),
    enabled: !!selectedRoute,
  })

  const checkpoints = selectedRoute?.checkpoints ?? []
  const scannedIds = new Set(report?.scans?.map(s => s.checkpoint_id) ?? [])

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold text-gray-800">Patrol Routes</h2>
      </div>

      {/* Filters */}
      <div className="flex gap-3">
        <select value={siteFilter} onChange={e => { setSiteFilter(e.target.value); setSelectedRoute(null) }}
          className="border border-gray-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500">
          <option value="">All sites</option>
          {sites?.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}
        </select>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Route list */}
        <div className="bg-white rounded-xl border border-gray-100 shadow-sm overflow-hidden">
          <div className="px-4 py-3 border-b border-gray-100">
            <h3 className="font-medium text-gray-800">Routes</h3>
          </div>
          {isLoading ? (
            <div className="p-4"><TableSkeleton rows={4} cols={2} /></div>
          ) : !routes?.length ? (
            <div className="p-8 text-center text-gray-400">No patrol routes configured.</div>
          ) : (
            <ul className="divide-y divide-gray-50">
              {routes.map(route => (
                <li key={route.id}
                  onClick={() => setSelectedRoute(route)}
                  className={`px-4 py-3 cursor-pointer hover:bg-gray-50 transition-colors flex items-center justify-between ${selectedRoute?.id === route.id ? 'bg-blue-50 border-l-2 border-blue-500' : ''}`}
                >
                  <div className="flex items-center gap-3">
                    <MapPin className="w-5 h-5 text-gray-400" />
                    <div>
                      <p className="font-medium text-gray-900">{route.name}</p>
                      <p className="text-xs text-gray-500">
                        {Array.isArray(route.checkpoints) ? route.checkpoints.length : '?'} checkpoints
                      </p>
                    </div>
                  </div>
                  <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${route.active ? 'bg-green-100 text-green-700' : 'bg-gray-100 text-gray-500'}`}>
                    {route.active ? 'Active' : 'Inactive'}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>

        {/* Route detail & report */}
        <div className="space-y-4">
          {!selectedRoute ? (
            <div className="bg-white rounded-xl border border-gray-100 shadow-sm p-10 text-center text-gray-400">
              Select a route to view today's progress
            </div>
          ) : (
            <>
              {/* Date picker + completion meter */}
              <div className="bg-white rounded-xl border border-gray-100 shadow-sm p-4">
                <div className="flex items-center justify-between mb-3">
                  <h3 className="font-medium text-gray-800">{selectedRoute.name}</h3>
                  <input type="date" value={reportDate} onChange={e => setReportDate(e.target.value)}
                    className="border border-gray-200 rounded-lg px-2 py-1 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500" />
                </div>
                {reportLoading ? (
                  <div className="h-4 bg-gray-100 rounded animate-pulse" />
                ) : report ? (
                  <>
                    <div className="flex items-center justify-between text-sm text-gray-600 mb-1">
                      <span>Completion</span>
                      <span className="font-semibold">{report.completion_pct.toFixed(0)}%</span>
                    </div>
                    <div className="w-full bg-gray-100 rounded-full h-2">
                      <div
                        className={`h-2 rounded-full transition-all ${report.completion_pct === 100 ? 'bg-green-500' : report.completion_pct > 50 ? 'bg-blue-500' : 'bg-orange-500'}`}
                        style={{ width: `${report.completion_pct}%` }}
                      />
                    </div>
                    <p className="text-xs text-gray-500 mt-1">
                      {report.scanned_checkpoints} / {report.total_checkpoints} checkpoints scanned
                    </p>
                  </>
                ) : null}
              </div>

              {/* Checkpoint list */}
              <div className="bg-white rounded-xl border border-gray-100 shadow-sm overflow-hidden">
                <div className="px-4 py-3 border-b border-gray-100">
                  <h3 className="font-medium text-gray-800">Checkpoints</h3>
                </div>
                <ul className="divide-y divide-gray-50">
                  {checkpoints.map((cp, idx) => {
                    const done = scannedIds.has(cp.id)
                    const scan = report?.scans?.find(s => s.checkpoint_id === cp.id)
                    return (
                      <li key={cp.id} className="px-4 py-3 flex items-center justify-between">
                        <div className="flex items-center gap-3">
                          <span className="w-6 h-6 rounded-full bg-gray-100 text-xs flex items-center justify-center text-gray-500 font-medium">
                            {idx + 1}
                          </span>
                          <div>
                            <p className="text-sm font-medium text-gray-900">{cp.name}</p>
                            {cp.sla_minutes && (
                              <p className="text-xs text-gray-400 flex items-center gap-1">
                                <Clock className="w-3 h-3" /> SLA {cp.sla_minutes}min
                              </p>
                            )}
                          </div>
                        </div>
                        <div className="flex items-center gap-2">
                          {done ? (
                            <>
                              <span className="text-xs text-gray-400">
                                {scan ? new Date(scan.scanned_at).toLocaleTimeString() : ''}
                              </span>
                              <CheckCircle className="w-5 h-5 text-green-500" />
                            </>
                          ) : (
                            <XCircle className="w-5 h-5 text-gray-300" />
                          )}
                        </div>
                      </li>
                    )
                  })}
                  {checkpoints.length === 0 && (
                    <li className="px-4 py-6 text-center text-gray-400 text-sm">No checkpoints defined.</li>
                  )}
                </ul>
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  )
}
