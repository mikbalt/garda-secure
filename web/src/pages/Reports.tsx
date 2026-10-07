import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import apiClient from '../api/client'
import { TableSkeleton } from '../components/LoadingSkeleton'
import type { Site } from '../types'
import { FileText, Download } from 'lucide-react'

interface MonthlyRecapRow {
  guard_id: string
  guard_name: string
  present: number
  absent: number
  late: number
  early_out: number
  total: number
}

interface DailySummary {
  date: string
  site_id: string
  counts: Record<string, number>
  total: number
}

export default function Reports() {
  const today = new Date().toISOString().split('T')[0]
  const currentMonth = today.slice(0, 7)

  const [siteID, setSiteID] = useState('')
  const [month, setMonth] = useState(currentMonth)
  const [date, setDate] = useState(today)

  const { data: sites } = useQuery<Site[]>({
    queryKey: ['sites'],
    queryFn: () => apiClient.get('/sites').then(r => r.data),
  })

  const { data: recap, isLoading: recapLoading } = useQuery<MonthlyRecapRow[]>({
    queryKey: ['monthly-recap', siteID, month],
    queryFn: () =>
      apiClient.get(`/attendance/monthly-recap?site_id=${siteID}&month=${month}`).then(r => r.data),
    enabled: !!siteID,
  })

  const { data: summary, isLoading: summaryLoading } = useQuery<DailySummary>({
    queryKey: ['daily-summary', siteID, date],
    queryFn: () =>
      apiClient.get(`/attendance/daily-summary?site_id=${siteID}&date=${date}`).then(r => r.data),
    enabled: !!siteID,
  })

  const exportCSV = () => {
    if (!recap?.length) return
    const headers = ['Guard', 'Present', 'Absent', 'Late', 'Early Out', 'Total']
    const rows = recap.map(r => [r.guard_name, r.present, r.absent, r.late, r.early_out, r.total])
    const csv = [headers, ...rows].map(r => r.join(',')).join('\n')
    const blob = new Blob([csv], { type: 'text/csv' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `attendance-${siteID}-${month}.csv`
    a.click()
    URL.revokeObjectURL(url)
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold text-gray-800">Reports</h2>
      </div>

      {/* Controls */}
      <div className="bg-white rounded-xl border border-gray-100 shadow-sm p-4">
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <div>
            <label className="block text-xs text-gray-500 mb-1">Site *</label>
            <select value={siteID} onChange={e => setSiteID(e.target.value)}
              className="w-full border border-gray-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500">
              <option value="">Select a site</option>
              {sites?.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}
            </select>
          </div>
          <div>
            <label className="block text-xs text-gray-500 mb-1">Month (monthly recap)</label>
            <input type="month" value={month} onChange={e => setMonth(e.target.value)}
              className="w-full border border-gray-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500" />
          </div>
          <div>
            <label className="block text-xs text-gray-500 mb-1">Date (daily summary)</label>
            <input type="date" value={date} onChange={e => setDate(e.target.value)}
              className="w-full border border-gray-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500" />
          </div>
        </div>
      </div>

      {!siteID && (
        <div className="bg-white rounded-xl border border-gray-100 shadow-sm p-10 text-center text-gray-400">
          Select a site to view reports
        </div>
      )}

      {siteID && (
        <>
          {/* Daily summary cards */}
          <div className="bg-white rounded-xl border border-gray-100 shadow-sm p-4">
            <h3 className="font-medium text-gray-800 mb-4 flex items-center gap-2">
              <FileText className="w-4 h-4" /> Daily Summary — {date}
            </h3>
            {summaryLoading ? (
              <div className="h-16 bg-gray-50 rounded animate-pulse" />
            ) : summary ? (
              <div className="grid grid-cols-2 sm:grid-cols-5 gap-3">
                {['present', 'absent', 'late', 'early_out', 'pending'].map(status => (
                  <div key={status} className="text-center p-3 bg-gray-50 rounded-lg">
                    <p className="text-2xl font-bold text-gray-900">{summary.counts[status] ?? 0}</p>
                    <p className="text-xs text-gray-500 capitalize">{status.replace('_', ' ')}</p>
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-sm text-gray-400">No data for this date.</p>
            )}
          </div>

          {/* Monthly recap table */}
          <div className="bg-white rounded-xl border border-gray-100 shadow-sm overflow-hidden">
            <div className="px-4 py-3 border-b border-gray-100 flex items-center justify-between">
              <h3 className="font-medium text-gray-800">Monthly Recap — {month}</h3>
              <button onClick={exportCSV}
                disabled={!recap?.length}
                className="flex items-center gap-1 text-sm text-blue-600 hover:text-blue-800 disabled:opacity-40">
                <Download className="w-4 h-4" /> Export CSV
              </button>
            </div>
            {recapLoading ? (
              <div className="p-6"><TableSkeleton rows={5} /></div>
            ) : !recap?.length ? (
              <div className="p-8 text-center text-gray-400">No attendance data for this month.</div>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead className="bg-gray-50 border-b border-gray-100">
                    <tr>
                      {['Guard', 'Present', 'Late', 'Early Out', 'Absent', 'Total Days'].map(h => (
                        <th key={h} className="text-left px-4 py-3 font-medium text-gray-600">{h}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-50">
                    {recap.map(r => (
                      <tr key={r.guard_id} className="hover:bg-gray-50">
                        <td className="px-4 py-3 font-medium text-gray-900">{r.guard_name}</td>
                        <td className="px-4 py-3 text-green-700 font-medium">{r.present}</td>
                        <td className="px-4 py-3 text-yellow-700">{r.late}</td>
                        <td className="px-4 py-3 text-orange-700">{r.early_out}</td>
                        <td className="px-4 py-3 text-red-700">{r.absent}</td>
                        <td className="px-4 py-3 text-gray-600">{r.total}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </>
      )}
    </div>
  )
}
