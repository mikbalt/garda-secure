import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import apiClient from '../api/client'
import { TableSkeleton } from '../components/LoadingSkeleton'
import type { AttendanceRecord, Guard, Site } from '../types'
import { UserCheck, Clock, AlertCircle, Filter } from 'lucide-react'

interface AttendanceFilters {
  siteId: string
  date: string
  status: string
}

const STATUS_COLORS: Record<string, string> = {
  present:   'bg-green-100 text-green-800',
  absent:    'bg-red-100 text-red-800',
  late:      'bg-yellow-100 text-yellow-800',
  early_out: 'bg-orange-100 text-orange-800',
  pending:   'bg-gray-100 text-gray-600',
}

export default function Attendance() {
  const today = new Date().toISOString().split('T')[0]
  const [filters, setFilters] = useState<AttendanceFilters>({
    siteId: '',
    date: today,
    status: '',
  })

  const { data: sites } = useQuery<Site[]>({
    queryKey: ['sites'],
    queryFn: () => apiClient.get('/sites').then(r => r.data),
  })

  const { data: guards } = useQuery<Guard[]>({
    queryKey: ['guards'],
    queryFn: () => apiClient.get('/guards').then(r => r.data),
  })

  const { data: records, isLoading } = useQuery<AttendanceRecord[]>({
    queryKey: ['attendance', filters],
    queryFn: () => {
      const params = new URLSearchParams()
      if (filters.siteId) params.set('site_id', filters.siteId)
      if (filters.date) params.set('date', filters.date)
      if (filters.status) params.set('status', filters.status)
      return apiClient.get(`/attendance?${params}`).then(r => r.data)
    },
    refetchInterval: 60_000,
  })

  const guardName = (id: string) =>
    guards?.find(g => g.id === id)?.name ?? id.slice(0, 8)

  const siteName = (id: string) =>
    sites?.find(s => s.id === id)?.name ?? id.slice(0, 8)

  const summary = {
    present:   records?.filter(r => r.status === 'present').length ?? 0,
    absent:    records?.filter(r => r.status === 'absent').length ?? 0,
    late:      records?.filter(r => r.status === 'late').length ?? 0,
    pending:   records?.filter(r => r.status === 'pending').length ?? 0,
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold text-gray-800">Attendance</h2>
        <span className="text-sm text-gray-500">Auto-refreshes every 60s</span>
      </div>

      {/* Summary cards */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
        {[
          { label: 'Present',   value: summary.present,  icon: UserCheck,     color: 'text-green-600'  },
          { label: 'Absent',    value: summary.absent,   icon: AlertCircle,   color: 'text-red-600'    },
          { label: 'Late',      value: summary.late,     icon: Clock,         color: 'text-yellow-600' },
          { label: 'Pending',   value: summary.pending,  icon: Clock,         color: 'text-gray-500'   },
        ].map(({ label, value, icon: Icon, color }) => (
          <div key={label} className="bg-white rounded-xl border border-gray-100 shadow-sm p-4 flex items-center gap-3">
            <Icon className={`w-8 h-8 ${color}`} />
            <div>
              <p className="text-2xl font-bold text-gray-900">{value}</p>
              <p className="text-sm text-gray-500">{label}</p>
            </div>
          </div>
        ))}
      </div>

      {/* Filters */}
      <div className="bg-white rounded-xl border border-gray-100 shadow-sm p-4">
        <div className="flex items-center gap-2 mb-3 text-sm font-medium text-gray-700">
          <Filter className="w-4 h-4" />
          Filters
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <div>
            <label className="block text-xs text-gray-500 mb-1">Date</label>
            <input
              type="date"
              value={filters.date}
              onChange={e => setFilters(f => ({ ...f, date: e.target.value }))}
              className="w-full border border-gray-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
          </div>
          <div>
            <label className="block text-xs text-gray-500 mb-1">Site</label>
            <select
              value={filters.siteId}
              onChange={e => setFilters(f => ({ ...f, siteId: e.target.value }))}
              className="w-full border border-gray-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
            >
              <option value="">All sites</option>
              {sites?.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}
            </select>
          </div>
          <div>
            <label className="block text-xs text-gray-500 mb-1">Status</label>
            <select
              value={filters.status}
              onChange={e => setFilters(f => ({ ...f, status: e.target.value }))}
              className="w-full border border-gray-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
            >
              <option value="">All statuses</option>
              <option value="present">Present</option>
              <option value="absent">Absent</option>
              <option value="late">Late</option>
              <option value="early_out">Early out</option>
              <option value="pending">Pending</option>
            </select>
          </div>
        </div>
      </div>

      {/* Table */}
      <div className="bg-white rounded-xl border border-gray-100 shadow-sm overflow-hidden">
        {isLoading ? (
          <div className="p-6"><TableSkeleton rows={6} /></div>
        ) : !records?.length ? (
          <div className="p-10 text-center text-gray-400">
            No attendance records for this date / filter.
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="bg-gray-50 border-b border-gray-100">
                <tr>
                  {['Guard', 'Site', 'Clock In', 'Clock Out', 'Status', 'Notes'].map(h => (
                    <th key={h} className="text-left px-4 py-3 font-medium text-gray-600">{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-50">
                {records.map(r => (
                  <tr key={r.id} className="hover:bg-gray-50 transition-colors">
                    <td className="px-4 py-3 font-medium text-gray-900">{guardName(r.guard_id)}</td>
                    <td className="px-4 py-3 text-gray-600">{siteName(r.site_id)}</td>
                    <td className="px-4 py-3 text-gray-600">
                      {r.clock_in_at ? new Date(r.clock_in_at).toLocaleTimeString() : '—'}
                    </td>
                    <td className="px-4 py-3 text-gray-600">
                      {r.clock_out_at ? new Date(r.clock_out_at).toLocaleTimeString() : '—'}
                    </td>
                    <td className="px-4 py-3">
                      <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${STATUS_COLORS[r.status] ?? 'bg-gray-100 text-gray-600'}`}>
                        {r.status.replace('_', ' ')}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-gray-400 text-xs">
                      {r.override_reason ?? ''}
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
