import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import apiClient from '../api/client'
import { TableSkeleton } from '../components/LoadingSkeleton'
import type { Incident, Site } from '../types'
import { AlertTriangle, Siren, Filter, RefreshCw } from 'lucide-react'

const SEVERITY_COLORS: Record<string, string> = {
  low:      'bg-blue-100 text-blue-800',
  medium:   'bg-yellow-100 text-yellow-800',
  high:     'bg-orange-100 text-orange-800',
  critical: 'bg-red-100 text-red-800',
}

const STATUS_COLORS: Record<string, string> = {
  open:        'bg-red-100 text-red-800',
  in_progress: 'bg-yellow-100 text-yellow-800',
  resolved:    'bg-green-100 text-green-800',
}

export default function Incidents() {
  const qc = useQueryClient()
  const [siteFilter, setSiteFilter] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [typeFilter, setTypeFilter] = useState('')

  const { data: sites } = useQuery<Site[]>({
    queryKey: ['sites'],
    queryFn: () => apiClient.get('/sites').then(r => r.data),
  })

  const { data: incidents, isLoading, refetch } = useQuery<Incident[]>({
    queryKey: ['incidents', siteFilter, statusFilter, typeFilter],
    queryFn: () => {
      const p = new URLSearchParams()
      if (siteFilter) p.set('site_id', siteFilter)
      if (statusFilter) p.set('status', statusFilter)
      if (typeFilter) p.set('type', typeFilter)
      return apiClient.get(`/incidents?${p}`).then(r => r.data)
    },
    refetchInterval: 30_000,
  })

  const updateStatus = useMutation({
    mutationFn: ({ id, status }: { id: string; status: string }) =>
      apiClient.put(`/incidents/${id}/status`, { status }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['incidents'] }),
  })

  const siteName = (id: string) => sites?.find(s => s.id === id)?.name ?? id.slice(0, 8)

  const counts = {
    open:     incidents?.filter(i => i.status === 'open').length ?? 0,
    critical: incidents?.filter(i => i.severity === 'critical').length ?? 0,
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold text-gray-800">Incidents</h2>
        <button onClick={() => refetch()} className="flex items-center gap-1 text-sm text-gray-500 hover:text-gray-700">
          <RefreshCw className="w-4 h-4" /> Refresh
        </button>
      </div>

      {/* Summary */}
      <div className="grid grid-cols-2 gap-4">
        <div className="bg-white rounded-xl border border-gray-100 shadow-sm p-4 flex items-center gap-3">
          <AlertTriangle className="w-8 h-8 text-red-500" />
          <div>
            <p className="text-2xl font-bold text-gray-900">{counts.open}</p>
            <p className="text-sm text-gray-500">Open incidents</p>
          </div>
        </div>
        <div className="bg-white rounded-xl border border-gray-100 shadow-sm p-4 flex items-center gap-3">
          <Siren className="w-8 h-8 text-red-600" />
          <div>
            <p className="text-2xl font-bold text-gray-900">{counts.critical}</p>
            <p className="text-sm text-gray-500">Critical severity</p>
          </div>
        </div>
      </div>

      {/* Filters */}
      <div className="bg-white rounded-xl border border-gray-100 shadow-sm p-4">
        <div className="flex items-center gap-2 mb-3 text-sm font-medium text-gray-700">
          <Filter className="w-4 h-4" /> Filters
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <select value={siteFilter} onChange={e => setSiteFilter(e.target.value)}
            className="border border-gray-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500">
            <option value="">All sites</option>
            {sites?.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}
          </select>
          <select value={statusFilter} onChange={e => setStatusFilter(e.target.value)}
            className="border border-gray-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500">
            <option value="">All statuses</option>
            <option value="open">Open</option>
            <option value="in_progress">In Progress</option>
            <option value="resolved">Resolved</option>
          </select>
          <select value={typeFilter} onChange={e => setTypeFilter(e.target.value)}
            className="border border-gray-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500">
            <option value="">All types</option>
            <option value="incident">Incident</option>
            <option value="defect">Defect</option>
            <option value="visitor">Visitor</option>
            <option value="sos">SOS</option>
          </select>
        </div>
      </div>

      {/* Table */}
      <div className="bg-white rounded-xl border border-gray-100 shadow-sm overflow-hidden">
        {isLoading ? (
          <div className="p-6"><TableSkeleton rows={5} /></div>
        ) : !incidents?.length ? (
          <div className="p-10 text-center text-gray-400">No incidents found.</div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="bg-gray-50 border-b border-gray-100">
                <tr>
                  {['Site', 'Type', 'Severity', 'Description', 'Status', 'Reported', 'Actions'].map(h => (
                    <th key={h} className="text-left px-4 py-3 font-medium text-gray-600">{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-50">
                {incidents.map(inc => (
                  <tr key={inc.id} className="hover:bg-gray-50 transition-colors">
                    <td className="px-4 py-3 text-gray-600">{siteName(inc.site_id)}</td>
                    <td className="px-4 py-3 font-medium text-gray-900 capitalize">{inc.type}</td>
                    <td className="px-4 py-3">
                      <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${SEVERITY_COLORS[inc.severity] ?? ''}`}>
                        {inc.severity}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-gray-600 max-w-xs truncate">{inc.description ?? '—'}</td>
                    <td className="px-4 py-3">
                      <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${STATUS_COLORS[inc.status] ?? ''}`}>
                        {inc.status.replace('_', ' ')}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-gray-500 text-xs">
                      {new Date(inc.created_at).toLocaleString()}
                    </td>
                    <td className="px-4 py-3">
                      {inc.status === 'open' && (
                        <button
                          onClick={() => updateStatus.mutate({ id: inc.id, status: 'in_progress' })}
                          className="text-xs text-blue-600 hover:text-blue-800 font-medium"
                        >
                          Assign
                        </button>
                      )}
                      {inc.status === 'in_progress' && (
                        <button
                          onClick={() => updateStatus.mutate({ id: inc.id, status: 'resolved' })}
                          className="text-xs text-green-600 hover:text-green-800 font-medium"
                        >
                          Resolve
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}
