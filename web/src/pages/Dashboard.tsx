import { useQuery } from '@tanstack/react-query'
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from 'recharts'
import { Users, MapPin, AlertTriangle, Radio } from 'lucide-react'
import { CardSkeleton } from '../components/LoadingSkeleton'
import apiClient from '../api/client'
import type { DashboardSummary } from '../types'

async function fetchSummary(): Promise<DashboardSummary> {
  const { data } = await apiClient.get<DashboardSummary>('/api/v1/dashboard/summary')
  return data
}

interface StatCardProps {
  label: string
  value: number | string
  sub?: string
  icon: React.ReactNode
  color: string
}

function StatCard({ label, value, sub, icon, color }: StatCardProps) {
  return (
    <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-5 flex items-start gap-4">
      <div className={`p-3 rounded-lg ${color}`}>{icon}</div>
      <div>
        <p className="text-sm text-gray-500">{label}</p>
        <p className="text-2xl font-bold text-gray-900 mt-0.5">{value}</p>
        {sub && <p className="text-xs text-gray-400 mt-0.5">{sub}</p>}
      </div>
    </div>
  )
}

export default function Dashboard() {
  const { data, isLoading, error } = useQuery({
    queryKey: ['dashboard-summary'],
    queryFn: fetchSummary,
    refetchInterval: 30_000, // refresh every 30s
  })

  return (
    <div className="space-y-6">
      <h2 className="text-xl font-semibold text-gray-800">Dashboard</h2>

      {/* Stat cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
        {isLoading ? (
          Array.from({ length: 4 }).map((_, i) => <CardSkeleton key={i} />)
        ) : error ? (
          <p className="col-span-4 text-red-500 text-sm">Failed to load summary data.</p>
        ) : (
          <>
            <StatCard
              label="Today's Attendance"
              value={`${data?.attendance_today ?? 0} / ${data?.attendance_total ?? 0}`}
              sub="Guards present today"
              icon={<Users className="w-5 h-5 text-blue-600" />}
              color="bg-blue-50"
            />
            <StatCard
              label="Active Patrols"
              value={data?.active_patrols ?? 0}
              sub="Routes in progress"
              icon={<MapPin className="w-5 h-5 text-green-600" />}
              color="bg-green-50"
            />
            <StatCard
              label="Open Incidents"
              value={data?.open_incidents ?? 0}
              sub="Requiring attention"
              icon={<AlertTriangle className="w-5 h-5 text-yellow-600" />}
              color="bg-yellow-50"
            />
            <StatCard
              label="SOS Alerts"
              value={data?.sos_alerts_unacknowledged ?? 0}
              sub="Unacknowledged"
              icon={<Radio className="w-5 h-5 text-red-600" />}
              color="bg-red-50"
            />
          </>
        )}
      </div>

      {/* Weekly attendance chart */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-5">
        <h3 className="text-sm font-semibold text-gray-700 mb-4">
          Weekly Attendance (last 7 days)
        </h3>
        {isLoading ? (
          <div className="h-48 animate-pulse bg-gray-100 rounded" />
        ) : (
          <ResponsiveContainer width="100%" height={240}>
            <BarChart data={data?.weekly_attendance ?? []}>
              <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
              <XAxis dataKey="date" tick={{ fontSize: 12 }} />
              <YAxis tick={{ fontSize: 12 }} />
              <Tooltip />
              <Legend />
              <Bar dataKey="present" name="Present" fill="#1e40af" radius={[4, 4, 0, 0]} />
              <Bar dataKey="absent" name="Absent" fill="#fca5a5" radius={[4, 4, 0, 0]} />
            </BarChart>
          </ResponsiveContainer>
        )}
      </div>
    </div>
  )
}
