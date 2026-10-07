import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import apiClient from '../api/client'
import { TableSkeleton } from '../components/LoadingSkeleton'
import type { Guard, Site } from '../types'

function GuardForm({
  onSubmit,
  onCancel,
  loading,
}: {
  onSubmit: (data: Partial<Guard>) => void
  onCancel: () => void
  loading: boolean
}) {
  const [form, setForm] = useState({ name: '', employee_id: '', role: 'guard', status: 'active' })

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className="bg-white dark:bg-gray-800 rounded-xl shadow-xl p-6 w-full max-w-md">
        <h3 className="text-lg font-semibold mb-4">Add Guard</h3>
        <div className="space-y-3">
          <div>
            <label className="block text-sm font-medium mb-1">Name *</label>
            <input
              className="w-full border border-gray-300 dark:border-gray-600 rounded-lg px-3 py-2 text-sm bg-white dark:bg-gray-700"
              value={form.name}
              onChange={e => setForm(f => ({ ...f, name: e.target.value }))}
              placeholder="Full name"
            />
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">Employee ID</label>
            <input
              className="w-full border border-gray-300 dark:border-gray-600 rounded-lg px-3 py-2 text-sm bg-white dark:bg-gray-700"
              value={form.employee_id}
              onChange={e => setForm(f => ({ ...f, employee_id: e.target.value }))}
              placeholder="e.g. EMP-001"
            />
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">Role</label>
            <select
              className="w-full border border-gray-300 dark:border-gray-600 rounded-lg px-3 py-2 text-sm bg-white dark:bg-gray-700"
              value={form.role}
              onChange={e => setForm(f => ({ ...f, role: e.target.value }))}
            >
              <option value="guard">Guard</option>
              <option value="supervisor">Supervisor</option>
              <option value="admin">Admin</option>
            </select>
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">Status</label>
            <select
              className="w-full border border-gray-300 dark:border-gray-600 rounded-lg px-3 py-2 text-sm bg-white dark:bg-gray-700"
              value={form.status}
              onChange={e => setForm(f => ({ ...f, status: e.target.value }))}
            >
              <option value="active">Active</option>
              <option value="inactive">Inactive</option>
            </select>
          </div>
        </div>
        <div className="flex justify-end gap-3 mt-6">
          <button
            onClick={onCancel}
            className="px-4 py-2 text-sm border border-gray-300 dark:border-gray-600 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-700"
          >
            Cancel
          </button>
          <button
            disabled={loading || !form.name}
            onClick={() => onSubmit(form)}
            className="px-4 py-2 text-sm bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50"
          >
            {loading ? 'Saving...' : 'Add Guard'}
          </button>
        </div>
      </div>
    </div>
  )
}

const roleColors: Record<string, string> = {
  guard: 'bg-blue-100 text-blue-800',
  supervisor: 'bg-purple-100 text-purple-800',
  admin: 'bg-orange-100 text-orange-800',
}

const statusColors: Record<string, string> = {
  active: 'bg-green-100 text-green-800',
  inactive: 'bg-gray-100 text-gray-800',
}

export default function Guards() {
  const [selectedSite, setSelectedSite] = useState('')
  const [showForm, setShowForm] = useState(false)
  const qc = useQueryClient()

  const { data: sites = [] } = useQuery<Site[]>({
    queryKey: ['sites'],
    queryFn: () => apiClient.get<Site[]>('/sites').then(r => r.data),
  })

  const { data: guards, isLoading } = useQuery<Guard[]>({
    queryKey: ['guards', selectedSite],
    queryFn: () =>
      apiClient
        .get<Guard[]>('/guards', { params: { site_id: selectedSite || undefined } })
        .then(r => r.data),
  })

  const createMutation = useMutation({
    mutationFn: (data: Partial<Guard>) => apiClient.post('/guards', data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['guards'] })
      setShowForm(false)
    },
  })

  const updateStatusMutation = useMutation({
    mutationFn: ({ id, status }: { id: string; status: string }) =>
      apiClient.put(`/guards/${id}`, { status }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['guards'] }),
  })

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-white">Guards</h1>
          <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
            Manage security personnel and their roles
          </p>
        </div>
        <button
          onClick={() => setShowForm(true)}
          className="flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg text-sm font-medium hover:bg-blue-700 transition-colors"
        >
          <span className="text-lg leading-none">+</span>
          Add Guard
        </button>
      </div>

      {/* Filters */}
      <div className="flex gap-3">
        <select
          className="border border-gray-200 dark:border-gray-700 rounded-lg px-3 py-2 text-sm bg-white dark:bg-gray-800 text-gray-700 dark:text-gray-300"
          value={selectedSite}
          onChange={e => setSelectedSite(e.target.value)}
        >
          <option value="">All Sites</option>
          {sites.map(s => (
            <option key={s.id} value={s.id}>
              {s.name}
            </option>
          ))}
        </select>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        {[
          { label: 'Total Guards', value: guards?.length ?? '–', color: 'blue' },
          {
            label: 'Active',
            value: guards?.filter(g => g.status === 'active').length ?? '–',
            color: 'green',
          },
          {
            label: 'Supervisors',
            value: guards?.filter(g => g.role === 'supervisor').length ?? '–',
            color: 'purple',
          },
          {
            label: 'Inactive',
            value: guards?.filter(g => g.status === 'inactive').length ?? '–',
            color: 'gray',
          },
        ].map(stat => (
          <div
            key={stat.label}
            className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-xl p-4"
          >
            <p className="text-sm text-gray-500 dark:text-gray-400">{stat.label}</p>
            <p className="text-2xl font-bold text-gray-900 dark:text-white mt-1">{stat.value}</p>
          </div>
        ))}
      </div>

      {/* Table */}
      <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-xl overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 dark:bg-gray-700/50">
              <tr>
                {['Name', 'Employee ID', 'Role', 'Status', 'Actions'].map(h => (
                  <th
                    key={h}
                    className="px-4 py-3 text-left text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wide"
                  >
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-700">
              {isLoading ? (
                <tr>
                  <td colSpan={5} className="px-4 py-8">
                    <TableSkeleton rows={5} cols={5} />
                  </td>
                </tr>
              ) : guards && guards.length > 0 ? (
                guards.map(guard => (
                  <tr key={guard.id} className="hover:bg-gray-50 dark:hover:bg-gray-700/30">
                    <td className="px-4 py-3 font-medium text-gray-900 dark:text-white">
                      {guard.name}
                    </td>
                    <td className="px-4 py-3 text-gray-500 dark:text-gray-400">
                      {guard.employee_id ?? '—'}
                    </td>
                    <td className="px-4 py-3">
                      <span
                        className={`inline-flex px-2 py-0.5 rounded-full text-xs font-medium ${roleColors[guard.role] ?? 'bg-gray-100 text-gray-800'}`}
                      >
                        {guard.role}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <span
                        className={`inline-flex px-2 py-0.5 rounded-full text-xs font-medium ${statusColors[guard.status] ?? 'bg-gray-100 text-gray-800'}`}
                      >
                        {guard.status}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <button
                        onClick={() =>
                          updateStatusMutation.mutate({
                            id: guard.id,
                            status: guard.status === 'active' ? 'inactive' : 'active',
                          })
                        }
                        className="text-xs text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300 font-medium"
                      >
                        {guard.status === 'active' ? 'Deactivate' : 'Activate'}
                      </button>
                    </td>
                  </tr>
                ))
              ) : (
                <tr>
                  <td colSpan={5} className="px-4 py-10 text-center text-gray-400 dark:text-gray-500">
                    No guards found.{' '}
                    <button
                      onClick={() => setShowForm(true)}
                      className="text-blue-600 hover:underline"
                    >
                      Add the first guard
                    </button>
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      {showForm && (
        <GuardForm
          onSubmit={data => createMutation.mutate(data)}
          onCancel={() => setShowForm(false)}
          loading={createMutation.isPending}
        />
      )}
    </div>
  )
}
